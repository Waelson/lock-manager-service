# Etapa 1: Build da aplicação em Golang
FROM golang:1.22 AS builder

# Define o diretório de trabalho
WORKDIR /app

# Copia os arquivos do projeto
COPY go.mod ./
RUN go mod download

# Copia o restante do código (cmd e static)
COPY . .

# Compila a aplicação com otimizações para produção
RUN CGO_ENABLED=0 GOOS=linux go build -o lock-manager-api ./cmd/main.go

# Etapa 2: Imagem final
FROM alpine:latest

# Define o diretório de trabalho
WORKDIR /root/

# Copia o binário gerado na etapa anterior
COPY --from=builder /app/lock-manager-api .

# Define o comando padrão para iniciar a aplicação
CMD ["./lock-manager-api"]