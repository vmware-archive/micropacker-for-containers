IMAGE = golang\:1\.10-alpine
BINARY = micropacker

docker-build:
	docker run -it --rm -u $$(id -u):$$(id -g) -v $$(pwd)/$(BINARY):/go/src/$(BINARY) -e "GOCACHE=off" -e "CGO_ENABLED=0" -e "GOOS=linux" -w /go/src/$(BINARY) $(IMAGE) go build -a -ldflags '-extldflags "-static"' .

clean:
	rm -f $(BINARY)/$(BINARY)
