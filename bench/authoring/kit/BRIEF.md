# Cliente: recepção da clínica odontológica

Entregue um agente Tessera para atender pacientes autenticados no chat,
em português brasileiro, com o acolhimento de uma boa recepcionista.
Precisamos remarcar consultas existentes do próprio paciente. A identidade
vem em caller.role = patient e caller.patientId. O contrato da agenda está
em api/clinic.openapi.json; o endereço do serviço é https://clinic.bench.

O paciente pode pedir um dia ou período, esclarecer dúvidas, corrigir sua
escolha ou desistir. Antes de alterar, deixe claro qual consulta será movida,
o horário atual e o novo, e obtenha confirmação explícita. Não altere
consultas de outra pessoa, invente vagas nem repita uma alteração por causa
de uma falha de comunicação ou mensagem repetida. Se não houver opção
adequada, explique. A clínica atende perguntas sobre endereço, convênios,
valores, horários e estacionamento com os dados do serviço. Pedidos de
cancelamento ou novo agendamento não podem ser executados pelo chat:
explique o limite e indique o contato fornecido pela clínica. Responda às
dúvidas sem perder uma remarcação em andamento e sem prometer algo fora
dessas capacidades.

Classificação de efeitos já aprovada pelo cliente para este projeto:
GETs são read; POST de remarcação é irreversible. Use esse contrato,
inclusive a consulta para verificar se uma alteração realmente ocorreu.
A agenda não oferece idempotência nem autorização por conta própria.
Dados de testes devem ser sintéticos. O serviço real não será acessado:
use os sandboxes documentados para testar as integrações.

Entregue recursos em authored/clinica, namespace clinica-bench, com Function
roteável, testes de conversa que cubram as regras e falhas, validação sem
erros e testes ao vivo e gravados aprovados. Use o CLI distribuído e
registre limitações no REPORT.md. Decida detalhes de implementação pelas
skills/documentação. Não há acesso ao código-fonte da plataforma.
