# RFC-0010: modo agente — um modelo planeja a conversa, o Tessera garante as ações

- **Estado:** padrão do traveler desde agency-005 (objetivos 65% no roteador → 77% no modo agente, 0 falhas de invariante); opcional para os demais agentes (ADR-0007). Medições: agency-004, agency-005.

## O problema

**Três estudos (agency-001..003) levaram o agente de viagens de 39% a 58%
dos objetivos**, sempre com zero ações indevidas. Cada rodada rendeu menos
que a anterior, e o motivo é a cauda longa: cada grupo novo de clientes
traz um jeito novo de pedir. Por exemplo:
- "reenvia o e-mail";
- "o mesmo uma semana depois";
- duas perguntas e uma cotação na mesma mensagem.

**O roteador escolhe uma Function por mensagem; o modo livre responde mas
não age.** Entre os dois fica justamente a conversa que um atendente
humano faz sem pensar: buscar, olhar o resultado, responder a outra
pergunta e seguir.

## A proposta

**Um modo opcional em que, a cada mensagem sem processo pendente, um
modelo (o Generate) planeja o turno em passos.** Em cada passo, ele:
1. vê a conversa recente, o que as ferramentas já devolveram neste turno
   e a lista de ferramentas;
2. escolhe entre:
   - **chamar uma Function** do menu, com os argumentos que já sabe;
   - **ler** uma ação só de leitura (reservas do cliente);
   - **buscar** nos documentos ou na web;
   - **responder** ao cliente.
3. até 5 passos por turno.

**Uma Function chamada roda como hoje:** entradas, perguntas,
confirmação, aprovação e idempotência.
- Se ela espera o cliente (pergunta ou confirmação), o turno acaba ali e
  a mensagem dela vai para o cliente.
- Se ela termina, o resultado volta ao planejador como observação, e ele
  pode continuar: responder a segunda pergunta, comparar, oferecer o
  próximo passo.

**Com um processo pendente, o Jev decide como hoje:**
- uma resposta vai para o processo;
- uma pergunta ou pedido paralelo vai para o planejador, não para o modo
  livre.

**Garantias, que não mudam:**
- **o planejador não age:** efeitos só acontecem dentro de Functions,
  com confirmação ou aprovação;
- **a resposta do planejador passa pelas mesmas checagens do modo livre:**
  - apoio nos fatos (o que as ferramentas devolveram, o que o agente já
    disse);
  - a trava de afirmações;
- **tudo é registrado no journal:** cada passo do planejador (a
  ferramenta, os argumentos, o resultado) e o turno como um todo;
- **as invariantes das EvalSuites valem igual.**

## Como vamos saber (agency-004)

- **Personas novas:** dev e teste selado.
- **Comparação:** o traveler de agora (D, roteador e modo livre) contra o
  mesmo traveler com o modo agente (E), com o mesmo juiz cego, os mesmos
  critérios de segurança e o custo à parte.
- **Primeiro com o modelo pequeno.** Um modelo maior só se o pequeno
  falhar, e registrando onde.

## Decisões

| # | Decisão | Por quê |
| --- | --- | --- |
| D1 | Planejador com saída JSON por esquema, não "tool calling" nativo | funciona com qualquer modelo do Generate, é validado pelo esquema e registrado como os outros |
| D2 | Functions como ferramentas, com os argumentos passados como entradas já dadas | reaproveita tudo que as Functions garantem; nada novo decide efeito |
| D3 | Jev continua nas pendências (resposta/pergunta/confirmação) | é a decisão medida e calibrada (freemode-005, preference-001) |
| D4 | Máximo de 5 passos por turno e US$ 0,01 de orçamento por turno | limita laços e custo |
