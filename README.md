This is the AWS provider used by Arcoloom.

## Instance lifecycle

The provider now exposes `StartInstance` and `StopInstance` RPCs for EC2 lifecycle management.

- `StartInstance` provisions an EC2 instance through Pulumi Automation API and accepts parameters such as `instance_type`, `market_type` (`on-demand` or `spot`), `ami`, subnet, security groups, key pair, user data, and tags.
- `StopInstance` tears down the Pulumi stack for the managed instance.

The concrete Pulumi-backed runner lives behind the `pulumi` build tag so the repository can still build before Pulumi dependencies are installed locally.

Example:

```bash
go build -tags pulumi ./cmd/arco-provider-aws
```

## Catalog and pricing

The provider now exposes standardized instance catalog and pricing APIs:

- `ListInstanceTypes`
- `GetInstanceTypeInfo`
- `GetInstancePrices`

AWS instance metadata is sourced from Arcoloom catalog data and cached on disk under `~/.arcoloom/instances/aws`.

- Catalog cache files:
  - `~/.arcoloom/instances/aws/catalog/instance_metadata.json`
  - `~/.arcoloom/instances/aws/catalog/instance_regions.json`
  - `~/.arcoloom/instances/aws/catalog/series_models.json`

`instance_regions.json` now also carries the standardized regional `on-demand` hourly price used by `GetInstancePrices`, so the provider no longer depends on the AWS public pricing offer files.

`GetInstancePrices` currently supports standardized `on-demand` pricing with these defaults when filters are omitted:

- `operating_system=Linux`
- `tenancy=Shared`
- `preinstalled_software=NA`
- `license_model=No License required`
- `currency=USD`
