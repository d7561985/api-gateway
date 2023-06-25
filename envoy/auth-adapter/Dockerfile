FROM golang:1.20-alpine

WORKDIR /app
#COPY xxx.crt /usr/local/share/ca-certificates/
RUN apk add ca-certificates git && update-ca-certificates
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o /app/auth-adapter .

CMD /app/auth-adapter