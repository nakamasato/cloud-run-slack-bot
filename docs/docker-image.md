# Docker Image

## Registry

### Docker Hub

https://hub.docker.com/r/nakamasato/cloud-run-slack-bot

### GitHub Container Registry (Not using)

As Cloud Run doesn't support ghcr.io, the image is stored in Docker Hub.

## Build

### Dockerfile (Not using)

GitHub Actions

```yaml
  build:
    needs:
      - test
      - golangci-lint
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}

      - name: Set image tag
        id: set_tag
        run: |
          if [ "$GITHUB_EVENT_NAME" == "pull_request" ]; then
            LATEST_SHA="${{ github.event.pull_request.head.sha }}"
            TAGS="pr-${{ github.event.number }}-${LATEST_SHA:0:7}"
          elif [ "$GITHUB_EVENT_NAME" == "push" ]; then
            TAGS="${GITHUB_SHA:0:7}"
          elif [ "$GITHUB_EVENT_NAME" == "release" ]; then
            TAGS="$GITHUB_REF_NAME"
          fi
          echo "TAGS=$TAGS" >> "$GITHUB_OUTPUT"

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: ${{ github.repository}}:${{ steps.set_tag.outputs.TAGS }}

      # - name: Docker Hub Description
      #   uses: peter-evans/dockerhub-description@v4
      #   with:
      #     username: ${{ secrets.DOCKERHUB_USERNAME }}
      #     password: ${{ secrets.DOCKERHUB_TOKEN }}
      #     repository: ${{ github.repository}}
```

1. https://github.com/marketplace/actions/build-and-push-docker-images

### Ko

```
KO_DOCKER_REPO=nakamasato/cloud-run-slack-bot
ko build . --tags latest --bare
```
