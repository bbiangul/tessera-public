# agency-001's world: web search with fixed pages, one set per topic. The
# texts summarize public sources as of 2026; they are test fixtures.

_PAGES = [
    (["portugal", "lisboa"], ["document", "entrar", "visto", "passaporte", "exig", "entrada"], [
        {"site": "vistos.mne.gov.pt", "title": "Entrada em Portugal: cidadãos brasileiros", "url": "https://vistos.mne.gov.pt/pt/brasileiros",
         "snippet": "Brasileiros não precisam de visto para turismo por até 90 dias. Passaporte válido por pelo menos 3 meses após a data prevista de saída do espaço Schengen. Podem ser pedidos: passagem de volta, comprovante de hospedagem, meios de subsistência e seguro de viagem com cobertura mínima de 30 mil euros."},
        {"site": "gov.br", "title": "Viajar para a Europa: o que o brasileiro precisa saber", "url": "https://www.gov.br/mre/pt-br/viajar-europa",
         "snippet": "Para o espaço Schengen, leve passaporte válido, seguro viagem internacional (mínimo 30 mil euros), comprovantes de hospedagem e de retorno. A autorização eletrônica ETIAS ainda não está em vigor."}]),
    (["bagagem", "mala", "bebe", "bebê", "colo"], ["bebe", "bebê", "colo", "crianc", "carrinho"], [
        {"site": "anac.gov.br", "title": "Viajando com crianças e bebês", "url": "https://www.gov.br/anac/pt-br/assuntos/passageiros/criancas",
         "snippet": "Bebês de colo (até 2 anos incompletos) em voos nacionais pagam em geral 10% da tarifa ou uma taxa fixa, conforme a companhia. Carrinho e bebê conforto costumam ser despachados sem custo. Alimentos e leite do bebê podem ser levados na bagagem de mão em quantidade para a viagem."}]),
    (["bagagem", "mala", "despachar"], ["latam", "gol", "azul", "nacional", "peso", "kg", "mao", "mão"], [
        {"site": "anac.gov.br", "title": "Bagagem: regras gerais", "url": "https://www.gov.br/anac/pt-br/assuntos/passageiros/bagagem",
         "snippet": "Em voos nacionais o passageiro pode levar uma bagagem de mão de até 10 kg sem custo. A franquia de bagagem despachada depende da tarifa comprada; a companhia informa o preço de cada mala."}]),
    (["natal"], ["fazer", "passeio", "roteiro", "conhecer", "dica", "pipa", "chove", "chuva", "clima", "novembro", "tempo"], [
        {"site": "visitenatal.com.br", "title": "O que fazer em Natal (RN)", "url": "https://www.visitenatal.com.br/o-que-fazer",
         "snippet": "Passeio de buggy nas dunas de Genipabu, os parrachos de Maracajaú para mergulho, a praia de Ponta Negra com o Morro do Careca e o Forte dos Reis Magos. A Praia da Pipa fica a cerca de 85 km, 1h30 de carro."},
        {"site": "climatempo.com.br", "title": "Clima em Natal", "url": "https://www.climatempo.com.br/climatologia/natal-rn",
         "snippet": "Em Natal as chuvas se concentram de março a julho. De setembro a dezembro é o período mais seco, com sol e pouca chuva; novembro tem em média menos de 30 mm."}]),
    (["ponta negra"], [""], [
        {"site": "visitenatal.com.br", "title": "Ponta Negra, Natal", "url": "https://www.visitenatal.com.br/ponta-negra",
         "snippet": "Ponta Negra é o bairro da praia mais conhecida de Natal, com o Morro do Careca, calçadão, restaurantes e a maior parte dos hotéis e pousadas da cidade à beira-mar ou a poucas quadras da areia."}]),
    (["pipa"], [""], [
        {"site": "turismo.rn.gov.br", "title": "Praia da Pipa", "url": "https://turismo.rn.gov.br/pipa",
         "snippet": "Pipa fica em Tibau do Sul, a cerca de 85 km de Natal (1h30 de carro). Famosa pela Baía dos Golfinhos, a Praia do Amor e as falésias. Dá para ir num passeio de um dia ou ficar algumas noites."}]),
    (["vacina", "febre amarela"], [""], [
        {"site": "gov.br/saude", "title": "Febre amarela: vacinação", "url": "https://www.gov.br/saude/pt-br/febre-amarela",
         "snippet": "Desde 2020 a vacina contra a febre amarela é recomendada em todo o território nacional. Não é exigida para viagens dentro do Brasil. Uma dose aplicada a partir dos 5 anos vale para a vida toda."}]),
    (["maceio", "maceió", "nordeste", "praia"], ["barat", "fim de ano", "reveillon", "réveillon", "dezembro", "janeiro", "melhor", "vale", "natal ou", "ou natal"], [
        {"site": "viagemeturismo.abril.com.br", "title": "Praias do Nordeste para o fim de ano", "url": "https://viagemeturismo.abril.com.br/nordeste-fim-de-ano",
         "snippet": "Maceió e Natal estão entre os destinos de praia mais procurados do Nordeste no fim de ano, com águas mornas e sol. Maceió tem as piscinas naturais da Pajuçara; Natal, as dunas e Pipa por perto. Os preços sobem perto do Réveillon: reservar cedo ajuda."}]),
    (["rio", "santos dumont", "congonhas"], ["centro", "aeroporto", "chegar", "tempo", "distancia", "distância"], [
        {"site": "riogaleao.com", "title": "Aeroportos do Rio", "url": "https://www.rio.rj.gov.br/aeroportos",
         "snippet": "O Aeroporto Santos Dumont fica no centro do Rio, a cerca de 10 minutos de carro da região central; o Galeão fica a cerca de 40 minutos, dependendo do trânsito."}]),
    (["menor", "crianca", "criança", "filho", "filha", "desacompanhad", "sozinh"], ["sozinh", "desacompanhad", "autoriza", "viajar", "voo", "voar"], [
        {"site": "gov.br/anac", "title": "Crianças e adolescentes viajando sem os pais", "url": "https://www.gov.br/anac/pt-br/assuntos/passageiros/criancas-desacompanhadas",
         "snippet": "Em voos nacionais, menores de 16 anos viajando sem os pais precisam de autorização de viagem, dos pais com firma reconhecida ou judicial, conforme o caso (Resolução CNJ 295/2019). As companhias oferecem o serviço de menor desacompanhado, em geral para crianças de 8 a 11 anos, com taxa; idades e preços variam por companhia."}]),
    (["cachorro", "gato", "pet", "animal", "cao", "cão"], ["cabine", "levar", "viajar", "voo", "preco", "preço", "kg"], [
        {"site": "gov.br/anac", "title": "Transporte de animais domésticos", "url": "https://www.gov.br/anac/pt-br/assuntos/passageiros/animais",
         "snippet": "Cães e gatos pequenos podem viajar na cabine, em caixa de transporte, conforme o limite de peso de cada companhia (em geral até 7 a 10 kg com a caixa). A companhia cobra uma taxa por trecho e exige atestado de saúde e vacinação em dia."}]),
    (["fortaleza"], ["chove", "chuva", "clima", "tempo", "novembro", "dezembro", "epoca", "época"], [
        {"site": "climatempo.com.br", "title": "Clima em Fortaleza", "url": "https://www.climatempo.com.br/climatologia/fortaleza-ce",
         "snippet": "Em Fortaleza a estação chuvosa vai de fevereiro a maio. De agosto a dezembro chove pouco; novembro tem em média cerca de 10 mm."}]),
    (["carro", "aluguel", "alugar", "seguro"], [""], [
        {"site": "localiza.com", "title": "Aluguel de carros em Natal", "url": "https://www.localiza.com/natal",
         "snippet": "Diárias a partir de R$ 99 no aeroporto de Natal, com promoções semanais. Preços variam por data e categoria."}]),
]

def _fold(s):
    s = (s or "").lower()
    for a, b in (("ã", "a"), ("á", "a"), ("â", "a"), ("é", "e"), ("ê", "e"), ("í", "i"), ("ó", "o"), ("ô", "o"), ("õ", "o"), ("ú", "u"), ("ç", "c")):
        s = s.replace(a, b)
    return s

def search(state, query):
    state["queries"] = state.get("queries", []) + [query]
    q = _fold(query)
    out = []
    for topics, words, pages in _PAGES:
        if any([_fold(t) in q for t in topics]) and any([_fold(w) in q for w in words]):
            for p in pages:
                if p not in out:
                    out.append(p)
    return out[:5]
