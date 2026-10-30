# Karyora — Guia do usuário

Este guia descreve o aplicativo de desktop Karyora como ele é entregue hoje. Ele declara apenas o que o produto realmente faz; capacidades que ainda não existem são listadas como limitações e nunca apresentadas como promessas. Este mesmo guia está disponível em 12 idiomas nesta pasta.

## 1. O que é o Karyora

- O Karyora é um aplicativo de desktop com prioridade local (local-first). Seus pacotes de conhecimento, jobs e configurações ficam armazenados na sua própria máquina.
- O produto funciona sem nenhum serviço de nuvem do projeto. O único tráfego de rede que ele pode gerar é o que você inicia: requisições aos seus provedores de IA, uma importação de pacote de conhecimento que você solicite e — se você configurá-lo — o canal de atualizações.
- Os recursos de IA usam as suas próprias contas de provedor (BYOK, "bring your own key"). Você configura suas próprias chaves de API e o provedor cobra você diretamente. O Karyora nunca revende o uso dos provedores nem atua como intermediário de seus pagamentos.

## 2. Versões comunitárias e oficiais

- O código-fonte do Karyora é publicado sob a licença Apache License 2.0. Você pode usá-lo, estudá-lo, modificá-lo e redistribuí-lo nos termos dessa licença, inclusive compilando-o por conta própria.
- O repositório público é `https://github.com/qira-dev/karyora`. Cada versão é marcada por uma tag `v*`, e a árvore de código-fonte dessa tag é o código-fonte da versão.
- Uma versão "oficial" é aquela construída pelo próprio pipeline de build do projeto em uma revisão com tag, com evidências registradas. As builds oficiais e as comunitárias têm os mesmos recursos; a diferença está em quem construiu o artefato e quem responde por ele, nunca no que o artefato pode fazer.
- Na v1 ainda não existe uma página oficial de downloads nem um canal oficial de atualizações. Até que sejam publicados, você pode obter o Karyora pelo repositório público e compilá-lo a partir do código-fonte.

## 3. Plataformas suportadas (v1)

| SO | CPU | Mínimo |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 ou mais recente |
| Windows 11 | ARM64 | Windows 11 para ARM |
| macOS | x86_64 | macOS 15 (Sequoia) ou mais recente |
| macOS | ARM64 | macOS 15 (Sequoia) ou mais recente, Apple Silicon |
| Linux | x86_64 | glibc 2.34 ou mais recente; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 ou mais recente; as mesmas famílias de distribuições |

Explicitamente sem suporte na v1: Windows 10; macOS 14 ou anterior; distribuições baseadas em musl, como o Alpine; qualquer combinação não listada acima. O suporte segue a janela de segurança de cada fornecedor e é verificado novamente a cada versão.

## 4. Pacotes e como verificá-los

- O conjunto de pacotes produzidos é: MSI do Windows, pkg do macOS, deb do Linux, rpm do Linux e um arquivo portátil do Linux — um artefato por arquitetura de CPU suportada.
- Cada pacote é produzido junto com um manifesto `SHA256SUMS` e registros de SBOM e proveniência por artefato. Para verificar um pacote: calcule o resumo sha256 do seu download e compare-o com a entrada do manifesto. Se o resumo não corresponder, não instale o pacote.
- Os pacotes da v1 não são assinados digitalmente — por design. As etapas de verificação de assinatura da plataforma (verificações de assinatura no nível do sistema operacional, chaves de repositório) serão documentadas junto com a primeira versão assinada. Até lá, a comparação de resumos acima é a verificação que você pode realizar.
- Os arquivos `LICENSE` e `NOTICE`, que contêm a licença do projeto e as atribuições de terceiros, fazem parte do repositório de código-fonte.

## 5. Atualizações

- O Karyora contém um mecanismo de atualização e, na v1, ele é entregue desativado por padrão. Nenhum canal de atualizações está configurado, de modo que uma verificação de atualizações informa uma recusa tipificada indicando que nenhum endpoint está configurado. Isso é intencional: o canal oficial de atualizações será ativado somente quando for publicado.
- Até lá, atualizar significa instalar o pacote mais novo sobre o anterior. Seus dados são preservados durante as atualizações (veja a seção 6).

## 6. Seus dados

- Todos os dados do produto ficam na sua máquina. O diretório de dados padrão é `%APPDATA%\Karyora\data` no Windows, `~/Library/Application Support/Karyora/data` no macOS e `~/.local/share/karyora/data` no Linux (a configuração XDG `XDG_DATA_HOME` é respeitada). Você pode escolher outro local com a flag `--data-dir`.
- O produto não faz backups programados. Para fazer backup, pare o produto e copie o diretório de dados. Restaurar significa devolver o diretório copiado ao lugar. Não há backup em nuvem.
- Desinstalar ou atualizar não apaga seus dados. Excluir dados é uma ação separada e explícita que você mesmo executa.
- Os dados em repouso não são criptografados. Proteja o diretório de dados com seus próprios meios, como criptografia de disco completo. Esta é uma limitação conhecida, declarada abertamente.
- As credenciais dos provedores (suas chaves de API) ficam somente no chaveiro do seu sistema operacional. Elas nunca são gravadas em arquivos, logs ou no conteúdo de requisições.

## 7. Privacidade

- O produto não inclui telemetria: nada é enviado aos servidores do projeto.
- Os diagnósticos são gerados localmente e, por padrão, preservam a privacidade; você controla se algo sai da sua máquina ou não.

## 8. Limitações conhecidas da v1

- A execução de plugins de terceiros está desativada.
- A operação em equipe e remota não está disponível.
- Transferir um job ativo de uma máquina para outra não é suportado.
- A sincronização de pacotes de conhecimento portáteis não está disponível.
- O agrupamento "Work" não existe.
- Não há assinaturas pagas nem direitos (entitlements): nada para comprar, sem planos pagos.
- Nenhuma promessa numérica de desempenho é feita.
- Os dados em repouso não são criptografados (veja a seção 6).
- Ainda não há página oficial de downloads nem canal de atualizações (veja as seções 2 e 5).

## 9. Licença

- O Karyora é publicado sob a licença Apache License 2.0. O texto completo da licença está no arquivo `LICENSE` e as atribuições de terceiros no arquivo `NOTICE`, ambos no repositório de código-fonte.
- Compilar o Karyora você mesmo, criar um fork ou redistribuí-lo conforme a licença preserva todos os direitos que a licença concede. Esse direito não depende de nenhuma compra, porque não há nada para comprar.

## 10. Referência da linha de comando

A ferramenta de linha de comando é o `uawp`.

| Comando | O que faz |
|---|---|
| `uawp start` | Inicia o serviço local. Um segundo start sobre o mesmo diretório de dados é recusado enquanto um já estiver em execução. |
| `uawp stop` | Para o serviço com um desligamento limitado e ordenado. |
| `uawp status` | Informa se o serviço está em execução. |
| `uawp doctor` | Executa verificações locais de saúde e diagnóstico. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Verifica, aplica ou informa sobre atualizações. Na v1, sem canal configurado, check e apply terminam com uma recusa tipificada indicando que nenhum endpoint está configurado; `status` informa o estado da atualização. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Envia um job durável, informa o status dele ou acompanha seus eventos. |
| `uawp credential store --provider <id> [--name api-key]` | Guarda a sua chave de API do provedor no chaveiro do sistema operacional. |
| `uawp credential status` | Informa se a credencial existe — nunca o seu valor. |
| `uawp credential remove` | Remove a credencial armazenada. |
| `uawp version` / `uawp help` | Exibe a versão ou a ajuda. |

Uma credencial armazenada, por si só, não abre nada: uma chamada real ao provedor ainda precisa passar pelas regras de tratamento de dados que o produto aplica a esse provedor.
