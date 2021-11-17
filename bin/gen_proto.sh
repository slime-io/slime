
# usage: sh <path_of_this_scipt> <path_of_proto_files>
# like:
#   cd to slime dir, exec: sh bin/gen_proto.sh api/v1alpha1/*.proto

# sudo apt install protobuf-compiler
# GO111MODULE=off go get github.com/gogo/protobuf/proto

GOPATH=${GOPATH:-$(go env GOPATH)}
for f in "$@"; do
  d=$(dirname $f)
  protoc -I="$d" -I="$(dirname $0)/../framework" \
    -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf/protobuf \
    --gogo_opt=paths=source_relative \
    --gogo_opt=Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types \
    --gogo_opt=Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types \
    --gogo_out="$d" "$f"
done
