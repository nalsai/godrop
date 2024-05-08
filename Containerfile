FROM docker.io/library/golang:1.21

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /godrop

EXPOSE 7598

CMD ["/godrop"]