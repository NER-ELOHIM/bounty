# bounty

Adaptador do `iode` que procura issues abertas com recompensa no GitHub, ranqueia
por aderência à sua stack e devolve as melhores.

Go com biblioteca padrão apenas: `net/http`, `encoding/json`, `context`. Sem
dependência externa, um binário, que é o que faz ele rodar igual na sua máquina e
num runner do GitHub Actions.

## O que ele faz, e o que não faz

Ele **caça e ranqueia**. Você decide e resolve.

Ele não escreve código, não abre pull request e não submete nada. Isso é o D4 do
`iode` — *"nada vindo de adaptador, de resposta HTTP ou de modelo de linguagem
chega ao executor"* — e o princípio 5 do README: *o LLM propõe, o motor não
obedece*. Um agente que lê issue da internet e submete patch é exatamente a
cadeia dado → execução que a arquitetura existe para impedir.

## Uso

```sh
echo '{"contract":1,"project":"bounties","path":"/tmp","since":null,
       "config":{"labels":["bounty"],"languages":["go","php","python"]}}' | bin/bounty
```

### Configuração

| chave | tipo | padrão | o que faz |
|---|---|---|---|
| `labels` | lista | `["bounty"]` | uma consulta por rótulo |
| `languages` | lista | — | sua stack; casar pontua +40 |
| `limit` | inteiro | 25 | quantos itens devolver |
| `per_page` | inteiro | 50 | resultados por consulta, máximo 100 |
| `timeout_seconds` | inteiro | 20 | timeout total |

### Credencial

O token sai de `IODE_GITHUB_TOKEN`. O motor repassa ao subprocesso tudo que
começa com `IODE_`, e nunca lê arquivo de credencial por conta própria — ver
`docs/SEGURANCA.md` do `iode`. Basta permissão de leitura pública.

Sem token o limite da **Search API** é 10 requisições por minuto; com token, 30.
Esse limite é bem mais baixo que o da API geral, então `labels` longo estoura a
cota e não coleta nada — o adaptador recusa mais de 30 rótulos com código 3.

### Ranqueamento

Determinístico, sem heurística de modelo: casamento de string e contagem.

| sinal | peso |
|---|---|
| casa com uma linguagem sua | +40 |
| valor ≥ US$ 1000 / ≥ 200 / > 0 | +40 / +25 / +10 |
| aberta esta semana / neste mês | +20 / +10 |
| aberta há mais de um ano | −20 |
| mais de 15 comentários | −15 |

O empate desempata pela mais recente.

### Códigos de saída

| código | quando |
|---|---|
| 0 | sucesso, mesmo com avisos |
| 1 | **todas** as consultas falharam; o motor tenta de novo |
| 2 | versão de contrato desconhecida |
| 3 | `stdin` vazio, JSON inválido, `since` sem offset, ou config inválida |

## Desenvolvimento

```sh
make check     # build + vet + test
make install   # copia para ~/.config/iode/adapters/bounty
```

Nenhum teste acessa a rede: as respostas do GitHub vêm de um `httptest` em
loopback. Cobertura acima de 80% nos três pacotes.
