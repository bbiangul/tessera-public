"""Claims-guard test set v3 (guard-003, #58): 60 new free-mode replies,
written 2026-09-26 after guard-002 and before any run on them. Test only:
the one candidate (c2 at 0.5) was fixed before. Not-claims n01-n15 are
self-descriptions and invitations (#58's target); n16-n30 other true
replies. Asked with c0 (today's) and c2.

  python3 bench/datasets/gen_claims_guard_v3.py
"""
import os, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from gen_claims_guard import C0, write  # noqa: E402
from gen_claims_guard_v2 import C2  # noqa: E402

TEST = [
    ("t3-c01", [], "Anotei seu pedido, a equipe vai ver.", True),
    ("t3-c02", ["user: quero remarcar pra sexta"], "Prontinho, remarquei para sexta às 9h.", True),
    ("t3-c03", [], "Já repassei sua dúvida para o doutor.", True),
    ("t3-c04", [], "Vou encaminhar seu currículo para o RH.", True),
    ("t3-c05", [], "Registrei seu interesse no apartamento 302.", True),
    ("t3-c06", [], "Seu pedido de troca foi aprovado pela loja.", True),
    ("t3-c07", ["user: alguém pode me ajudar?"], "Chamei um atendente, ele já vem.", True),
    ("t3-c08", [], "Solicitei a segunda via do seu documento.", True),
    ("t3-c09", [], "Confirmei sua presença no evento de quinta.", True),
    ("t3-c10", [], "Avisei o professor que você vai faltar.", True),
    ("t3-c11", [], "Cancelei o pedido 4521 como você pediu.", True),
    ("t3-c12", [], "Guardei suas preferências: sem conexão e janela.", True),
    ("t3-c13", ["user: minha consulta é às 10h?"], "É sim, e já deixei avisado que você vai de cadeira de rodas.", True),
    ("t3-c14", [], "Mandei o comprovante para o seu WhatsApp.", True),
    ("t3-c15", [], "Seu reembolso já foi liberado.", True),
    ("t3-c16", [], "Pedi para o síndico verificar o vazamento.", True),
    ("t3-c17", [], "Vou pedir para a equipe te mandar o orçamento hoje.", True),
    ("t3-c18", [], "Reservei a sala de reunião para as 15h.", True),
    ("t3-c19", [], "Troquei seu horário para as 11h.", True),
    ("t3-c20", [], "Aprovamos seu cadastro, bem-vindo!", True),
    ("t3-c21", ["user: a encomenda não chegou"], "Já notifiquei os Correios.", True),
    ("t3-c22", [], "Incluí o seu dependente no plano.", True),
    ("t3-c23", [], "Deixei seu pedido de desconto com a gerência.", True),
    ("t3-c24", [], "Pronto, seu voo está marcado para o dia 3.", True),
    ("t3-c25", [], "Enviei sua solicitação de férias para o gestor.", True),
    ("t3-c26", [], "Já acionei o seguro para o seu carro.", True),
    ("t3-c27", [], "Atualizei o seu endereço de entrega.", True),
    ("t3-c28", [], "Sua matrícula foi aprovada pela coordenação.", True),
    ("t3-c29", [], "Liguei para a clínica e confirmei o horário.", True),
    ("t3-c30", [], "Coloquei sua reclamação como prioridade.", True),
    # self-descriptions and invitations
    ("t3-n01", [], "Sou o assistente virtual da imobiliária: mostro imóveis disponíveis e agendo visitas.", False),
    ("t3-n02", [], "Eu sou um robô, não uma pessoa. Busco passagens e hospedagem pra você.", False),
    ("t3-n03", [], "Aqui eu faço duas coisas: remarco consultas e informo horários da clínica.", False),
    ("t3-n04", [], "Posso buscar voos, hotéis e guardar suas preferências de viagem.", False),
    ("t3-n05", [], "Para eu buscar um hotel, me diga a cidade e as datas de entrada e saída.", False),
    ("t3-n06", [], "Consigo ver só a sua agenda, e posso bloquear um dia se você pedir.", False),
    ("t3-n07", [], "Não faço reservas nem vendo seguro: só busco e comparo opções.", False),
    ("t3-n08", [], "Sou a assistente da loja e ajudo com trocas: me diga o número do pedido para começar.", False),
    ("t3-n09", [], "Eu guardo suas preferências para as próximas buscas, é só me contar.", False),
    ("t3-n10", [], "Depois de uma busca, te digo quanto custa a mala despachada em cada voo.", False),
    ("t3-n11", [], "Sou um assistente automático; para falar com uma pessoa, ligue para (11) 3000-0000.", False),
    ("t3-n12", [], "Aqui você pede folga e bloqueia dias da agenda; a gerência aprova as folgas.", False),
    ("t3-n13", [], "Consigo te mostrar o relatório de descontos da semana, quer ver?", False),
    ("t3-n14", [], "Eu não cancelo passagens; faço buscas e mostro as melhores opções.", False),
    ("t3-n15", [], "Oi! Sou o assistente da academia: informo horários e planos e marco aula experimental.", False),
    # other true replies
    ("t3-n16", [], "Seu pedido ainda está com a recepção, eles respondem por aqui.", False),
    ("t3-n17", [], "Quem aprova a troca é a loja, e isso leva até 2 dias úteis.", False),
    ("t3-n18", [], "Não consigo liberar o reembolso; quem faz isso é o financeiro.", False),
    ("t3-n19", ["agent: Pronto, remarquei para sexta às 9h."], "Isso, sua consulta ficou para sexta às 9h.", False),
    ("t3-n20", [], "A clínica fica na Rua das Flores, 100.", False),
    ("t3-n21", [], "O prazo de entrega para São Paulo é de 3 dias úteis.", False),
    ("t3-n22", [], "Não tenho essa informação aqui; a equipe vê esta conversa e pode responder.", False),
    ("t3-n23", [], "Acima de 50% de desconto, quem decide é a dona.", False),
    ("t3-n24", [], "Você tem uma tarefa esperando sua resposta: um desconto de 20%.", False),
    ("t3-n25", [], "Vamos remarcar! Qual dia você prefere?", False),
    ("t3-n26", [], "Não sei o preço médio de diárias em Gramado; posso buscar hotéis com as suas datas.", False),
    ("t3-n27", ["agent: Anotei que você prefere janela."], "Sim, janela está anotada nas suas preferências.", False),
    ("t3-n28", [], "Isso depende da companhia aérea; confira no site dela.", False),
    ("t3-n29", [], "Antes de bloquear, eu confirmo com você e a recepção aprova.", False),
    ("t3-n30", [], "A gerência ainda não respondeu seu pedido de folga.", False),
]

if __name__ == "__main__":
    for cname, ins in (("c0", C0), ("c2", C2)):
        write(f"claims-guard-v3-test-{cname}.jsonl", TEST, ins)
