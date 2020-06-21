module github.com/mellow-io/v2ray-core

require (
	github.com/golang/mock v1.2.0
	github.com/golang/protobuf v1.2.1-0.20190205222052-c823c79ea157
	github.com/google/go-cmp v0.2.0
	github.com/miekg/dns v1.1.22
	github.com/oschwald/maxminddb-golang v1.5.0
	github.com/stretchr/testify v1.4.0 // indirect
	go.starlark.net v0.0.0-20190225160109-1174b2613e82
	golang.org/x/crypto v0.0.0-20190923035154-9ee001bba392
	golang.org/x/net v0.0.0-20191021144547-ec77196f6094
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	google.golang.org/grpc v1.18.0
	h12.io/socks v1.0.0
)

replace v2ray.com/core => /opt/go/src/github.com/mellow-io/v2ray-core

replace github.com/eycorsican/go-tun2socks => /opt/go/src/github.com/mellow-io/go-tun2socks

go 1.13
