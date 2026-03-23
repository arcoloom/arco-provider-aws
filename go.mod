module github.com/arcoloom/arco-provider-aws

go 1.26.1

require (
	github.com/arcoloom/arco-proto v0.0.0-00010101000000-000000000000
	github.com/aws/aws-sdk-go-v2 v1.41.4
	github.com/aws/aws-sdk-go-v2/config v1.32.1
	github.com/aws/aws-sdk-go-v2/credentials v1.19.1
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.263.0
	github.com/aws/aws-sdk-go-v2/service/ssm v1.68.3
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.1
	github.com/aws/smithy-go v1.24.2
	google.golang.org/grpc v1.79.2
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.14 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/pricing v1.40.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.9 // indirect
	go.opentelemetry.io/otel v1.41.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.41.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)

replace github.com/arcoloom/arco-proto => ../arco-proto
