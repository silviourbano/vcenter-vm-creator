# Makefile para vcenter-vm-creator (Go)

BINARY_NAME=vcenter-vm-creator

.PHONY: all build run tidy vendor clean

# 🏗️ Compila o projeto (default)
all: build

# 🔨 Compila o binário principal a partir do main.go
build:
	go build -o $(BINARY_NAME) main.go

# 🚀 Compila e executa o binário
run: build
	./$(BINARY_NAME)

# 🧹 Organiza e atualiza as dependências do Go
 tidy:
	go mod tidy

# 📦 Baixa as dependências para a pasta vendor
vendor:
	go mod vendor

# 🧽 Remove o binário gerado
clean:
	rm -f $(BINARY_NAME)
