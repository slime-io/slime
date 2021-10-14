for f in "$@"; do
  d=$(dirname $f)
  protoc -I="$d" -I=../slime-framework -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf/protobuf --gogo_opt=paths=source_relative --gogo_opt=Mgoogle/protobuf/duration.proto=github.com/gogo/protobuf/types --gogo_out="$d" "$f"
done
