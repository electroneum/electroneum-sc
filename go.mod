module github.com/electroneum/electroneum-sc

go 1.22

toolchain go1.23.0

require (
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v0.3.0
	github.com/VictoriaMetrics/fastcache v1.6.0
	github.com/aws/aws-sdk-go-v2 v1.2.0
	github.com/aws/aws-sdk-go-v2/config v1.1.1
	github.com/aws/aws-sdk-go-v2/credentials v1.1.1
	github.com/aws/aws-sdk-go-v2/service/route53 v1.1.1
	github.com/btcsuite/btcd/btcec/v2 v2.2.0
	github.com/cespare/cp v0.1.0
	github.com/cloudflare/cloudflare-go v0.14.0
	github.com/consensys/gnark-crypto v0.4.1-0.20210426202927-39ac3d4b3f1f
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set v1.8.0
	github.com/docker/docker v1.6.2
	github.com/dop251/goja v0.0.0-20220405120441-9037c2b61cbf
	github.com/edsrzf/mmap-go v1.0.0
	github.com/fatih/color v1.7.0
	github.com/fjl/gencodec v0.0.0-20220412091415-8bb9e558978c
	github.com/fsnotify/fsnotify v1.6.0
	github.com/gballet/go-libpcsclite v0.0.0-20190607065134-2772fd86a8ff
	github.com/go-stack/stack v1.8.0
	github.com/golang-jwt/jwt/v4 v4.3.0
	github.com/golang/protobuf v1.4.3
	github.com/golang/snappy v0.0.4
	github.com/google/gofuzz v1.1.1-0.20200604201612-c04b05f3adfa
	github.com/google/uuid v1.2.0
	github.com/gorilla/websocket v1.4.2
	github.com/graph-gophers/graphql-go v1.3.0
	github.com/hashicorp/go-bexpr v0.1.10
	github.com/hashicorp/golang-lru v0.5.5-0.20210104140557-80c98217689d
	github.com/holiman/bloomfilter/v2 v2.0.3
	github.com/holiman/uint256 v1.2.0
	github.com/huin/goupnp v1.0.3
	github.com/influxdata/influxdb v1.8.3
	github.com/influxdata/influxdb-client-go/v2 v2.4.0
	github.com/jackpal/go-nat-pmp v1.0.2
	github.com/jedisct1/go-minisign v0.0.0-20190909160543-45766022959e
	github.com/julienschmidt/httprouter v1.2.0
	github.com/karalabe/usb v0.0.3-0.20230711191512-61db3e06439c
	github.com/mattn/go-colorable v0.1.8
	github.com/mattn/go-isatty v0.0.12
	github.com/naoina/toml v0.1.2-0.20170918210437-9fafd6967416
	github.com/olekukonko/tablewriter v0.0.5
	github.com/peterh/liner v1.1.1-0.20190123174540-a2c9a5303de7
	github.com/prometheus/tsdb v0.7.1
	github.com/rs/cors v1.7.0
	github.com/shirou/gopsutil v3.21.4-0.20210419000835-c7a38de76ee5+incompatible
	github.com/status-im/keycard-go v0.0.0-20190316090335-8537d3370df4
	github.com/stretchr/testify v1.8.1
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/tyler-smith/go-bip39 v1.0.1-0.20181017060643-dbb3b84ba2ef
	golang.org/x/crypto v0.11.0
	golang.org/x/mobile v0.0.0-20190719004257-d2bd2a29d028
	golang.org/x/sync v0.3.0
	golang.org/x/sys v0.10.0
	golang.org/x/term v0.10.0
	golang.org/x/text v0.11.0
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	golang.org/x/tools v0.11.0
	gopkg.in/karalabe/cookiejar.v2 v2.0.0-20150724131613-8dcd6a7f4951
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce
	gopkg.in/urfave/cli.v1 v1.20.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v0.21.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v0.8.3 // indirect
	github.com/StackExchange/wmi v0.0.0-20180116203802-5d049714c4a6 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.0.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.0.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.1.1 // indirect
	github.com/aws/smithy-go v1.1.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/deepmap/oapi-codegen v1.8.2 // indirect
	github.com/dlclark/regexp2 v1.4.1-0.20201116162257-a2a8dda75c91 // indirect
	github.com/garslo/gogen v0.0.0-20170306192744-1d203ffc1f61 // indirect
	github.com/go-ole/go-ole v1.2.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/influxdata/line-protocol v0.0.0-20210311194329-9aa0e372d097 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/mitchellh/pointerstructure v1.2.0 // indirect
	github.com/naoina/go-stringutil v0.1.0 // indirect
	github.com/opentracing/opentracing-go v1.1.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tklauser/go-sysconf v0.3.5 // indirect
	github.com/tklauser/numcpus v0.2.2 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/net v0.12.0 // indirect
	google.golang.org/protobuf v1.23.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
