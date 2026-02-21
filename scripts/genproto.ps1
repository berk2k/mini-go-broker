$OutDir = "api/proto/gen"

if (!(Test-Path $OutDir)) {
    New-Item -ItemType Directory -Path $OutDir | Out-Null
}

protoc `
  -I api/proto `
  --go_out=$OutDir --go_opt=paths=source_relative `
  --go-grpc_out=$OutDir --go-grpc_opt=paths=source_relative `
  api/proto/broker.proto