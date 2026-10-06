package main

import (
	"flag"
	"log"
	"os"

	function "github.com/crossplane/function-sdk-go"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
)

type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
}

func main() {
	insecure := flag.Bool("insecure", false, "disable mTLS for local development")
	tlsDir := flag.String("tls-certs-dir", os.Getenv("TLS_SERVER_CERTS_DIR"), "mTLS certificate directory")
	flag.Parse()
	if err := function.Serve(&Function{}, function.MTLSCertificates(*tlsDir), function.Insecure(*insecure)); err != nil {
		log.Fatal(err)
	}
}
