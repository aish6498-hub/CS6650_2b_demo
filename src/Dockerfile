FROM --platform=linux/amd64 golang:1.27
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app-server
EXPOSE 8080
CMD ["/app-server"]
