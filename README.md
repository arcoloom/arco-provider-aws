This is the AWS provider used by Arcoloom.

## Instance lifecycle

The provider now exposes `StartInstance` and `StopInstance` RPCs for EC2 lifecycle management.

- `StartInstance` provisions an EC2 instance directly through the AWS SDK and accepts parameters such as `instance_type`, `market_type` (`on-demand` or `spot`), `ami`, subnet, security groups, key pair, user data, and tags.
- If `ami` is omitted, the provider resolves the latest Debian 13 AMI from Debian's public SSM parameters based on the instance architecture.
- `StartInstance` requires an explicit `region`; it no longer falls back to `scope.region` or `us-east-1`.
- `StopInstance` finds EC2 instances tagged with the request `stack_name` and terminates them through the AWS SDK, scanning either `options.regions` or all enabled account regions.

## Catalog and pricing

The provider now exposes standardized instance catalog and pricing APIs:

- `ListRegions`
- `ListAvailabilityZones`
- `ListInstanceTypes`
- `GetInstanceTypeInfo`
- `GetInstancePrices`

`ListRegions` and `ListAvailabilityZones` use live EC2 account metadata to discover which regions and availability zones are currently enabled for the caller.

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
