# godrop

I was annoyed by the lack of a simple service that I could run on my own server, to let other people upload files to me. So I made one in one day.

Nextcloud file drop is great but I somewhat dislike Nextcloud and wanted something simpler.

## Usage

You can either use the provided container image or build the server yourself with `go build`.

Here is my docker-compose.yml file:

```yml
version: "3.9"

services:
  godrop:
    image: ghcr.io/nalsai/godrop
    container_name: godrop
    restart: unless-stopped
    #ports:
      #- 7598:7598
    volumes:
      - ./data:/app/uploads
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.godrop.rule=Host(`godrop.example.com`)"
      - "traefik.http.routers.godrop.entrypoints=websecure"
      - "traefik.http.routers.godrop.tls=true"
      - "traefik.http.routers.godrop.tls.certresolver=letsencrypt"
      - "traefik.http.services.godrop.loadbalancer.server.port=7598"
    networks:
      - traefik

networks:
  traefik:
    external: true
```

## License

This project is licensed under the [MIT License](LICENSE).
