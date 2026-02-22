### Building and running your application

When you're ready, start your application by running:
`docker compose up --build`.

Your application will be available at http://localhost:8080.

### Deploying your application to the cloud

First, build your image, e.g.: `docker build -t myapp .`.
If your cloud uses a different CPU architecture than your development
machine (e.g., you are on a Mac M1 and your cloud provider is amd64),
you'll want to build the image for that platform, e.g.:
`docker build --platform=linux/amd64 -t myapp .`.

Then, push it to your registry, e.g. `docker push myregistry.com/myapp`.

Consult Docker's [getting started](https://docs.docker.com/go/get-started-sharing/)
docs for more detail on building and pushing.

### References
* [Docker's Go guide](https://docs.docker.com/language/golang/)

### Generate Go protobuf files (dev)

Protobuf tool image is located at `tools/protobuf/Dockerfile`.

Run protobuf generation with the dedicated dev tools profile:

`docker compose -f compose-dev.yaml --profile tools run --rm --build protobuf`

This generates Go files for:
- `pb/order_engine/order.proto`
- `pb/order_engine/order_engine.proto`
