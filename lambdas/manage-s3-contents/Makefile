test: 
	go vet

local:
	GOARCH=amd64 CGO_ENABLED=0 GOOS=linux go build -tags lambda.norpc -o bootstrap

build:
	GOARCH=amd64 CGO_ENABLED=0 GOOS=linux go build -tags lambda.norpc -o bootstrap

update:
	go get -u
	go mod tidy

.PHONY: test build update