package aws

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/arcoloom/arco-provider-aws/internal/provider"
)

type routedAWSAccount struct {
	AccountID   string
	Name        string
	Credentials provider.AWSCredentials
}

func routeAWSAccounts(credentials provider.Credentials, scope provider.ConnectionScope) []routedAWSAccount {
	if len(credentials.AWSAccounts) != 0 {
		items := make([]routedAWSAccount, 0, len(credentials.AWSAccounts))
		for _, account := range credentials.AWSAccounts {
			items = append(items, routedAWSAccount{
				AccountID:   resolveInternalAccountID(account.Name, account.Credentials, scope),
				Name:        strings.TrimSpace(account.Name),
				Credentials: account.Credentials,
			})
		}
		slices.SortFunc(items, func(left, right routedAWSAccount) int {
			return strings.Compare(left.AccountID, right.AccountID)
		})
		return items
	}
	if credentials.AWS == nil {
		return nil
	}
	return []routedAWSAccount{{
		AccountID:   resolveInternalAccountID("", *credentials.AWS, scope),
		Credentials: *credentials.AWS,
	}}
}

func routeAWSAccount(credentials provider.Credentials, requestedAccountID string, scope provider.ConnectionScope) (routedAWSAccount, error) {
	accounts := routeAWSAccounts(credentials, scope)
	if len(accounts) == 0 {
		return routedAWSAccount{}, fmt.Errorf("aws iam credentials are required")
	}

	requestedAccountID = strings.TrimSpace(requestedAccountID)
	if requestedAccountID == "" {
		if len(accounts) == 1 {
			return accounts[0], nil
		}
		return routedAWSAccount{}, fmt.Errorf("account_id is required when multiple provider accounts are configured")
	}

	for _, account := range accounts {
		if account.AccountID == requestedAccountID {
			return account, nil
		}
	}
	return routedAWSAccount{}, fmt.Errorf("unknown account_id %q for this provider runtime", requestedAccountID)
}

func resolveInternalAccountID(name string, credentials provider.AWSCredentials, scope provider.ConnectionScope) string {
	parts := []string{
		strings.TrimSpace(name),
		strconvBool(credentials.UseDefaultCredentialsChain),
		strings.TrimSpace(credentials.Profile),
		strings.TrimSpace(credentials.AccessKeyID),
		strings.TrimSpace(credentials.RoleARN),
		strings.TrimSpace(credentials.ExternalID),
		strings.TrimSpace(credentials.RoleSessionName),
		strings.TrimSpace(credentials.SourceIdentity),
		strings.TrimSpace(scope.Endpoint),
		strings.TrimSpace(scope.EndpointRegion),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "acct_" + hex.EncodeToString(digest[:8])
}

func strconvBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
