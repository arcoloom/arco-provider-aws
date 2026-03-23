package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awspricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

const (
	defaultPricingAPIRegion = "us-east-1"
	onDemandPriceBatchSize  = 100
)

type awsOnDemandPriceDocument struct {
	Product struct {
		SKU        string            `json:"sku"`
		Attributes map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			EffectiveDate   string `json:"effectiveDate"`
			PriceDimensions map[string]struct {
				Unit         string            `json:"unit"`
				Description  string            `json:"description"`
				PricePerUnit map[string]string `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

func (s *Service) getOnDemandPrices(
	ctx context.Context,
	credentials provider.Credentials,
	scope provider.ConnectionScope,
	snapshot catalogSnapshot,
	region string,
	instanceTypes []string,
	operatingSystem string,
	tenancy string,
	preinstalledSoftware string,
	licenseModel string,
	currency string,
) ([]provider.InstancePrice, error) {
	accounts := routeAWSAccounts(credentials, scope)
	if len(accounts) == 0 {
		return nil, fmt.Errorf("aws iam credentials are required")
	}

	items := dedupeNormalizedStrings(instanceTypes)
	if len(items) == 0 {
		items = marketInstanceTypesForRegion(snapshot, region)
	}
	if len(items) == 0 {
		return nil, nil
	}

	regionName := strings.TrimSpace(snapshot.regionNames[region])
	if regionName == "" {
		regionName = findRegionNameAcrossSnapshot(snapshot, region)
	}
	if regionName == "" {
		return nil, fmt.Errorf("region %q is not available in catalog metadata", region)
	}

	cfg, err := s.clientFactory.NewConfig(ctx, accounts[0].Credentials, defaultPricingAPIRegion, scope.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("build pricing config: %w", err)
	}

	client := s.clientFactory.NewPricing(cfg)
	results := make(map[string]provider.InstancePrice, len(items))
	for _, batch := range chunkStrings(items, onDemandPriceBatchSize) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		batchResults, err := fetchOnDemandPriceBatch(ctx, client, region, regionName, batch, operatingSystem, tenancy, preinstalledSoftware, licenseModel, currency)
		if err != nil {
			return nil, err
		}
		for instanceType, item := range batchResults {
			results[instanceType] = item
		}
	}

	prices := make([]provider.InstancePrice, 0, len(results))
	for _, instanceType := range items {
		if item, ok := results[normalizeToken(instanceType)]; ok {
			prices = append(prices, item)
		}
	}

	slices.SortFunc(prices, func(a, b provider.InstancePrice) int {
		if cmp := strings.Compare(a.InstanceType, b.InstanceType); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Price, b.Price); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.SKU, b.SKU)
	})

	return prices, nil
}

func fetchOnDemandPriceBatch(
	ctx context.Context,
	client pricingAPI,
	region string,
	regionName string,
	instanceTypes []string,
	operatingSystem string,
	tenancy string,
	preinstalledSoftware string,
	licenseModel string,
	currency string,
) (map[string]provider.InstancePrice, error) {
	paginator := awspricing.NewGetProductsPaginator(client, &awspricing.GetProductsInput{
		ServiceCode: awsv2.String("AmazonEC2"),
		Filters: []pricingtypes.Filter{
			termFilter("instanceType", strings.Join(instanceTypes, ","), pricingtypes.FilterTypeAnyOf),
			termFilter("location", regionName, pricingtypes.FilterTypeTermMatch),
			termFilter("locationType", "AWS Region", pricingtypes.FilterTypeTermMatch),
			termFilter("operatingSystem", operatingSystem, pricingtypes.FilterTypeTermMatch),
			termFilter("tenancy", tenancy, pricingtypes.FilterTypeTermMatch),
			termFilter("preInstalledSw", preinstalledSoftware, pricingtypes.FilterTypeTermMatch),
			termFilter("licenseModel", licenseModel, pricingtypes.FilterTypeTermMatch),
			termFilter("capacitystatus", "Used", pricingtypes.FilterTypeTermMatch),
		},
		MaxResults: awsv2.Int32(onDemandPriceBatchSize),
	})

	results := make(map[string]provider.InstancePrice, len(instanceTypes))
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get on-demand prices: %w", err)
		}
		for _, raw := range page.PriceList {
			item, ok, err := parseOnDemandPrice(raw, region, currency)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			results[normalizeToken(item.InstanceType)] = item
		}
	}

	return results, nil
}

func parseOnDemandPrice(raw string, region string, currency string) (provider.InstancePrice, bool, error) {
	var document awsOnDemandPriceDocument
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return provider.InstancePrice{}, false, fmt.Errorf("decode on-demand price document: %w", err)
	}

	instanceType := strings.TrimSpace(document.Product.Attributes["instanceType"])
	if instanceType == "" {
		return provider.InstancePrice{}, false, nil
	}

	regionCode := strings.TrimSpace(document.Product.Attributes["regionCode"])
	if regionCode != "" && regionCode != region {
		return provider.InstancePrice{}, false, nil
	}

	price, billingUnit, description, effectiveAt, ok := extractOnDemandPriceDimension(document.Terms.OnDemand, currency)
	if !ok {
		return provider.InstancePrice{}, false, nil
	}

	return provider.InstancePrice{
		InstanceType:         instanceType,
		Region:               provider.Region{Code: region, Name: strings.TrimSpace(document.Product.Attributes["location"])},
		PurchaseOption:       provider.PurchaseOptionOnDemand,
		OperatingSystem:      strings.TrimSpace(document.Product.Attributes["operatingSystem"]),
		Tenancy:              strings.TrimSpace(document.Product.Attributes["tenancy"]),
		PreinstalledSoftware: strings.TrimSpace(document.Product.Attributes["preInstalledSw"]),
		LicenseModel:         strings.TrimSpace(document.Product.Attributes["licenseModel"]),
		BillingUnit:          billingUnit,
		Currency:             currency,
		Price:                price,
		EffectiveAt:          effectiveAt,
		SKU:                  strings.TrimSpace(document.Product.SKU),
		Description:          description,
	}, true, nil
}

func extractOnDemandPriceDimension(
	terms map[string]struct {
		EffectiveDate   string `json:"effectiveDate"`
		PriceDimensions map[string]struct {
			Unit         string            `json:"unit"`
			Description  string            `json:"description"`
			PricePerUnit map[string]string `json:"pricePerUnit"`
		} `json:"priceDimensions"`
	},
	currency string,
) (string, string, string, time.Time, bool) {
	for _, term := range terms {
		effectiveAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(term.EffectiveDate))
		for _, dimension := range term.PriceDimensions {
			price := strings.TrimSpace(dimension.PricePerUnit[currency])
			if price == "" {
				continue
			}
			unit := strings.TrimSpace(dimension.Unit)
			if unit != "" && unit != "Hrs" {
				continue
			}
			return price, unit, strings.TrimSpace(dimension.Description), effectiveAt, true
		}
	}

	return "", "", "", time.Time{}, false
}

func termFilter(field string, value string, filterType pricingtypes.FilterType) pricingtypes.Filter {
	return pricingtypes.Filter{
		Field: awsv2.String(field),
		Type:  filterType,
		Value: awsv2.String(value),
	}
}

func dedupeNormalizedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		normalized := normalizeToken(trimmed)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func findRegionNameAcrossSnapshot(snapshot catalogSnapshot, region string) string {
	for _, regions := range snapshot.regionsByType {
		if name := findRegionName(regions, region); name != "" {
			return name
		}
	}
	return ""
}
