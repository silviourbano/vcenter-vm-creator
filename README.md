# vcenter-vm-creator

Ferramenta em Go para clonar máquinas virtuais no vCenter, baseada em template, com configuração flexível via linha de comando e variáveis de ambiente.

## Como funciona

- Conecta-se ao vCenter usando as credenciais fornecidas.
- Permite inspecionar a configuração de rede de uma VM existente (`-inspect-vm`).
- Realiza o clone de uma VM a partir de um template, configurando rede, datastore, cluster, resource pool e pasta de destino.

## Variáveis de ambiente

Essas variáveis podem ser definidas em um arquivo `.env` ou exportadas no ambiente:

- `VMWARE_URL` — URL de conexão com o vCenter (obrigatório)
- `VMWARE_USERNAME` — Usuário do vCenter (obrigatório)
- `VMWARE_PASSWORD` — Senha do vCenter (obrigatório)
- `VCENTER_INSECURE` — Se "true", permite conexão insegura (SSL não verificado)

## Parâmetros de linha de comando (flags)

Todos os parâmetros possuem valores padrão e podem ser sobrescritos:

- `-datacenter` — Nome do Datacenter alvo (default: `DC-PRODUCAO`)
- `-cluster` — Nome do Cluster alvo (default: `PRD-DB-LINUX`)
- `-template` — Nome do template da VM ou caminho completo no inventário (default: `OracleDB_OL7_Template`)
- `-vm-name` — Nome da nova máquina virtual (default: `my-new-vm`)
- `-datastore` — Nome do Datastore alvo (default: `TESP5STG1P00002_VMWARE_DB_LNX_34`)
- `-network` — Nome do rótulo da rede (default: `TSILV_CSILV_ate_network-production`)
- `-resource-pool` — Nome do Resource Pool (opcional, default: vazio)
- `-folder` — Nome da pasta de destino da VM (default: `TSILV_CSILV_ate`)
- `-inspect-vm` — Nome de uma VM existente para inspecionar a configuração de rede (opcional, default: vazio)

## Exemplos de uso

### Clonar uma VM (usando todos os padrões):
Usando Makefile:
```sh
make run
```
Ou executando o binário diretamente:
```sh
make build
./vcenter-vm-creator
```

### Clonar uma VM especificando nome, pasta e datastore:
Usando Makefile:
```sh
make run ARGS="-vm-name minha-vm -folder MINHA_PASTA -datastore MEU_DATASTORE"
```
Ou executando o binário diretamente:
```sh
make build
./vcenter-vm-creator -vm-name minha-vm -folder MINHA_PASTA -datastore MEU_DATASTORE
```

### Inspecionar a rede de uma VM existente:
Usando Makefile:
```sh
make run ARGS="-inspect-vm nome-da-vm-existente"
```
Ou executando o binário diretamente:
```sh
make build
./vcenter-vm-creator -inspect-vm nome-da-vm-existente
```

## Observações
- O script só realiza o clone se o parâmetro `-inspect-vm` não for utilizado.
- Para melhor performance, forneça o caminho completo do template na flag `-template` (ex: `/DC-PRODUCAO/vm/pasta/template`).
- O script só suporta redes do tipo Distributed Virtual Portgroup.

---

Para dúvidas ou sugestões, abra uma issue!
