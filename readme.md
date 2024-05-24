# build
go install golang.org/x/tools/cmd/stringer@latest
go generate

# run
go run . info -r all