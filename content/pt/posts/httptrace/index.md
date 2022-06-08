---
title: "Go: Tracing requisições HTTP"
date: 2022-06-06T07:07:15-03:00
draft: false
---

O protocolo HTTP é rápido, seguro e confiável. Porém, ele necessita de outros protocolos e serviços para funcionar corretamente e, **quando as coisas não vão bem, faz-se necessário ter acesso a informações detalhadas sobre o tempo gasto em cada etapa**.

As etapas para se realizar uma chamada HTTP são as seguintes:

1. Tradução de DNS
   1. O cliente envia uma consulta DNS para o servidor de DNS.
   1. O servidor de DNS responde com o IP para o nome.
1. Conexão TCP
   1. O cliente envia o pacote SYN.
   1. O servidor Web responde com pacote SYN-ACK.
   1. O cliente estabelece a conexão _(triple handshake)_ com o pacote ACK.
1. Envio de dados
   1. O cliente envia a requisição HTTP para o servidor Web.
1. Espera
   1. O cliente espera até que o servidor Web responda a requisição.
   1. O servidor Web processa a requisição e envia a resposta para o cliente que recebe os cabeçalhos de resposta HTTP e o conteúdo.
1. Carregamento
   1. O cliente carrega o conteúdo da resposta.
1. Fechamento
   1. O cliente envia um pacote FIN para fechar a conexão TCP.

Este é apenas um dos possíveis casos para uma requisição HTTP, pois não estamos abordando conexões persistentes, _pool_ de conexões ou outras funcionalidades do protocolo.

Go possui o pacote **net/http/httptrace** para que possamos coletar informações detalhadas sobre uma requisição HTTP e será o assunto deste artigo.

Após tudo isso dito, **quanto tempo leva-se para traduzir o nome para IP**. Baixe o arquivo [simple-main.go](simple-main.go).

```go
func main() {
	var start, dns time.Time
	var took, dnsTook time.Duration

	ct := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
         dns = time.Now()
      },
		DNSDone:  func(info httptrace.DNSDoneInfo) {
         dnsTook = time.Since(dns)
      },
	}
	req, _ := http.NewRequest(http.MethodGet, "https://httpbin.org/", nil)
	ctCtx := httptrace.WithClientTrace(req.Context(), ct)
	req = req.WithContext(ctCtx)
	start = time.Now()
	_, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	took = time.Since(start)
	fmt.Printf("total %dms, dns %dms\n",
      took.Milliseconds(), dnsTook.Milliseconds()
   )
}
```

Rodando o código acima você verá um resultado similar ao abaixo:

```bash
MacBook-Pro:hi thiagocarvalho$ go run main.go
total 612ms, dns 2ms
MacBook-Pro:hi thiagocarvalho$
```

O tempo total da requisição foi de 612 ms enquanto o DNS levou 2ms.

## Cliente HTTP

Antes de começarmos a falar sobre _httptrace_, vamos relembrar um pouco sobre o cliente do pacote [_net/http_](https://pkg.go.dev/net/http) ou mais especificamente sobre o tipo [_http.Client_](https://pkg.go.dev/net/http#Client).

```go
type Client struct {
   // Transport specifies the mechanism by which individual
   // HTTP requests are made.
   // If nil, DefaultTransport is used.
   Transport RoundTripper
   ...
   Timeout time.Duration
}
```

Para este artigo iremos nos ater apenas no campo _Transport_ da _struct_, que é um [_http.RoundTripper_](https://pkg.go.dev/net/http#RoundTripper), que nada mais é que uma interface para uma função que recebe um ponteiro para [_http.Request_](https://pkg.go.dev/net/http#Request) e retorna um ponteiro para [_http.Response_](https://pkg.go.dev/net/http#Response) e um erro. Isto é bastante conveniente já que basicamente tudo em uma chamada cliente HTTP envolve uma requisição, uma resposta e se houve algum erro no processo.

## RoundTripper

Segundo a documentação do Go um _RoundTrip_ é `"the ability to execute a single HTTP transaction, obtaining the Response for a given Request."`. De um modo simplista podemos dizer que _RoundTrip_ nada mais é que um _middleware_ da sua chamada HTTP. Você geralmente não precisa se preocupar com isso até o momento que você tem que adicionar um comportamento padrão para **todas** as chamadas feitas por sua aplicação, como por exemplo servir uma página do _cache_ ao invés de ir buscar no servidor ou implementar _retries_.

O _DefaultRoundTrip_ é a seguinte variável:

```go
var DefaultTransport RoundTripper = &Transport{
	Proxy: ProxyFromEnvironment,
	DialContext: defaultTransportDialContext(&net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}),
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}
```

O transporte de uma chamada HTTP é basicamente o que controla a comunicação entre o cliente e o servidor, ou seja, transportar os dados da melhor forma possível. Não irei entrar em detalhes sobre tudo que envolve a camada de transporte do protocolo pois seria muito grande e não tenho todo o conhecimento necessário sem fazer muita pesquisa, mas você pode inferir muita coisa apenas prestando atenção aos nomes pois são bastante explícitos. **Um especial agradecimento a todos que investem tempo em decidir o melhor nome para cada coisa no seu código**.

Voltando ao assunto, o _DefaultTransport_ gerencia as conexões de rede, ou seja, é responsável por criar novas conexões conforme necessário criando um _cache_ para serem reutilizadas nas próximas requisições, como também honra as variáveis de ambiente _$HTTP_PROXY_ e _$NO_PROXY_.

Por fim sobre transporte, vale a pena citar algo que está na documentação sobre _http.Response.Body_:

```go
...
// The default HTTP client's Transport
// may not reuse HTTP/1.x "keep-alive" TCP connections
// if the Body is not read to completion and closed.
...
Body io.ReadCloser
```

A conexão não será reutilizada até que o _body_ seja lido!

## HTTPTrace

No Go 1.7 o pacote [_net/http/httptrace_](https://pkg.go.dev/net/http/httptrace) foi criado para coleta de informações através do ciclo de vida de uma chamada cliente HTTP. O pacote é pequeno e nos apresenta um novo tipo [_ClientTrace_](https://pkg.go.dev/net/http/httptrace#ClientTrace) e uma função [_WithClientTrace_](https://pkg.go.dev/net/http/httptrace#WithClientTrace).

### Tipo ClientTrace

```go
type ClientTrace struct {
   ...
   GetConn func(hostPort string)
   ...
   DNSStart func(DNSStartInfo)
   ...
   ConnectDone func(network, addr string, err error)
   ...
   TLSHandshakeStart func()
   ...
   WroteRequest func(WroteRequestInfo)
}
```

O novo tipo introduzido pelo pacote é basicamente uma coleção de funções que são injetadas (_hooks_) por vários _http.RoundTriper_.

Você é responsável por escrever as funções, o que você ganha do pacote é a injeção automática da função na hora certa mais dados. Por exemplo, quando a biblioteca vai enviar o pacote para resolver o nome via DNS automaticamente o campo _DNSStart_ é injetado e você rebece [_DNSStartInfo_](https://pkg.go.dev/net/http/httptrace#DNSStartInfo) e quando a resposta é recebida do servidor automaticamente o campo _DNSDone_ é injetado e você recebe [_DNSDoneInfo_](https://pkg.go.dev/net/http/httptrace#DNSDoneInfo).

### Função WithClientTrace

```go
old := ContextClientTrace(ctx)
trace.compose(old)

ctx = context.WithValue(ctx, clientEventContextKey{}, trace)
if trace.hasNetHooks() {
   ...
   ctx = context.WithValue(ctx, nettrace.TraceKey{}, nt)
}
return ctx
```

A função _WithClientTrace_ faz o seguinte:

1. Copia o contexto antigo.
1. Cria um novo contexto baseado no contexto pai e adiciona os valores de _trace_ no contexto.
1. Verifica se existe algum _hook_ relativo a camada de rede para ser injetado no objeto _ClientTrace_.
1. Se encontrado, as funções são injetadas no contexto.

### Trace e http.Client

Vamos agora criar o código para realizar _trace_ de uma chamada HTTP e nos dar informações detalhadas sobre o tempo gasto em cada etapa.

1. Precisamos escrever as funções para criar o _ClientTrace_. Ex.:

   ```go
   func dnsStart(info httptrace.DNSStartInfo) {
      fmt.Printf("quering %q to DNS\n", info.Host)
   }
   func dnsDone(info httptrace.DNSDoneInfo) {
      fmt.Println("DNS info",info)
   }
   ```

1. Instanciar o objeto _ClientTrace_ com as funções criadas.

   ```go
   clientTrace := &httptrace.ClientTrace{
      DNSStart: dnsStart,
      DNSDone:  dnsDone,
   }
   ```

1. Instanciar o objeto da requisição.

   ```go
   req, _ := http.NewRequest(http.MethodGet, "https://httpbin.org/redirect-to?url=https://example.com&status_code=307", nil)
   ```

1. Instanciar um novo contexto com o _trace_.

   ```go
   clientTraceCtx := httptrace.WithClientTrace(req.Context(), clientTrace)
   ```

1. Associar o novo contexto ao objeto requisição.

   ```go
   req = req.WithContext(clientTraceCtx)
   ```

1. Realizar a chamada HTTP.

   ```go
   resp, err := http.DefaultClient.Do(req)
   if err != nil {
   	log.Fatal(err)
   }
   ```

1. Baixar o conteúdo da resposta e fechar o _reader_.

   ```go
   _, err = io.Copy(io.Discard, resp.Body)
   if err != nil {
   	log.Fatal(err)
   }
   resp.Body.Close()
   ```

Acesse o código completo no [link](complete-main.go), leia-o com atenção e rode do seu computador. Você deverá ver algo semelhante ao conteúdo abaixo.

```text
2022/06/03 17:18:15 [TRACE] - starting to create conn to "releases.ubuntu.com:443"
2022/06/03 17:18:15 [TRACE] - quering "releases.ubuntu.com" to DNS
2022/06/03 17:18:15 [TRACE] - ip addresses:
2022/06/03 17:18:15 [TRACE] - - 2620:2d:4000:1::1a
2022/06/03 17:18:15 [TRACE] - - 2001:67c:1562::28
2022/06/03 17:18:15 [TRACE] - - 2620:2d:4000:1::17
2022/06/03 17:18:15 [TRACE] - - 2001:67c:1562::25
2022/06/03 17:18:15 [TRACE] - - 91.189.91.123
2022/06/03 17:18:15 [TRACE] - - 91.189.91.124
2022/06/03 17:18:15 [TRACE] - - 185.125.190.40
2022/06/03 17:18:15 [TRACE] - - 185.125.190.37
2022/06/03 17:18:15 [TRACE] - starting tcp connection to "[2620:2d:4000:1::1a]:443"
2022/06/03 17:18:15 [TRACE] - tcp connection created to [2620:2d:4000:1::1a]:443, err: <nil>
2022/06/03 17:18:15 [TRACE] - starting tls negotiation
2022/06/03 17:18:15 [TRACE] - tls negotiated to "releases.ubuntu.com", error: <nil>
2022/06/03 17:18:15 [TRACE] - connection established. reused: false idle: false idle time: 0ms
2022/06/03 17:18:15 [TRACE] - sending header "Host" and value [releases.ubuntu.com]
2022/06/03 17:18:15 [TRACE] - sending header "User-Agent" and value [Go-http-client/1.1]
2022/06/03 17:18:15 [TRACE] - sending header "Accept-Encoding" and value [gzip]
2022/06/03 17:18:15 [TRACE] - headers written
2022/06/03 17:18:15 [TRACE] - starting to wait for server response
2022/06/03 17:18:16 [TRACE] - got first response byte
2022/06/03 17:18:17 [TRACE] - put conn idle, err: <nil>
Statistics in ms
DNS     Connect TLS     Send    Wait    Transfer        Total
269.808 214.251 226.744 0.005   213.591 1605.033        2530.235
```

## Conclusão

HTTP _tracing_ é um nova _feature_ muito valiosa em Go para aqueles que querem ter informações de latência para chamadas HTTP e escrever ferramentas para _troubleshooting_ de trafego de saída.
