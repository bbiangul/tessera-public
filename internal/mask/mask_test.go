package mask

import (
	"reflect"
	"strings"
	"testing"
)

func TestText(t *testing.T) {
	cases := map[string]string{
		"meu email é ana.souza+x@exemplo.com.br":   "meu email é " + Email,
		"CPF 123.456.789-09 e 12345678909":         "CPF " + CPF + " e " + CPF,
		"CNPJ 12.345.678/0001-95":                  "CNPJ " + CNPJ,
		"cartão 4111 1111 1111 1111 ok":            "cartão " + Card + " ok",
		"cartão 4111-1111-1111-1111":               "cartão " + Card,
		"liga (11) 98765-4321 ou +55 11 987654321": "liga " + Phone + " ou " + Phone,
		"fixo 11 3456-7890":                        "fixo " + Phone,
		// Not personal data: must survive.
		"pedido 1234567890123 (não passa em Luhn)": "pedido 1234567890123 (não passa em Luhn)",
		"às 14:30 do dia 23/09/2026":               "às 14:30 do dia 23/09/2026",
		"R$ 1.250,00 em 3x":                        "R$ 1.250,00 em 3x",
		"quero remarcar a consulta":                "quero remarcar a consulta",
	}
	for in, want := range cases {
		if got := Text(in); got != want {
			t.Errorf("Text(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestMaskWalksValues(t *testing.T) {
	in := map[string]any{
		"messages": []any{"email a@b.co", int64(5)},
		"n":        3.5,
	}
	want := map[string]any{
		"messages": []any{"email " + Email, int64(5)},
		"n":        3.5,
	}
	if got := Default.Mask(in); !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
	if in["messages"].([]any)[0] != "email a@b.co" {
		t.Error("Mask must not modify its input")
	}
}

// Typed string lists and maps are masked too: a []string used to pass
// through unmasked (found by the voice's tests).
func TestMaskTypedCollections(t *testing.T) {
	m := Patterns{}
	l := m.Mask([]string{"meu email é ana@x.com"}).([]string)
	mm := m.Mask(map[string]string{"k": "ligue 11 98765-4321"}).(map[string]string)
	if strings.Contains(l[0], "ana@x.com") || strings.Contains(mm["k"], "98765") {
		t.Errorf("%q %q", l, mm)
	}
}

// #55: a Brazilian number written as bare digits (a WhatsApp ID) is a
// phone; other long digit runs are not.
func TestBareBrazilianNumbers(t *testing.T) {
	for in, want := range map[string]string{
		"Desconto de 15% para o paciente 5511911112222?": "Desconto de 15% para o paciente [PHONE]?",
		"whatsapp 5521987654321.":                        "whatsapp [PHONE].",
		"fixo 551130304040":                              "fixo [PHONE]",
		// Not phones: an invalid area code, ids, a longer run, a 12-digit
		// run whose subscriber part starts with 9 (not a landline).
		"id 5500000000005":       "id 5500000000005",
		"pedido 000023495472":    "pedido 000023495472",
		"conta 5511911112222333": "conta 5511911112222333",
		"protocolo 551198765432": "protocolo 551198765432",
		"número 100000000000000": "número 100000000000000",
	} {
		if got := Text(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestFind(t *testing.T) {
	cases := []struct{ kind, text, want string }{
		{"email", "É esse mesmo: marina.costa@example.com. Pode mandar.", "marina.costa@example.com"},
		{"email", "meu email é o de sempre", ""},
		{"cpf", "cpf 123.456.789-09 ok", "123.456.789-09"},
		{"phone", "me liga no (11) 98765-4321", "(11) 98765-4321"},
		{"phone", "5511987654321", "5511987654321"},
		{"name", "Marina", ""},
	}
	for _, c := range cases {
		if got := Find(c.kind, c.text); got != c.want {
			t.Errorf("Find(%s, %q) = %q, want %q", c.kind, c.text, got, c.want)
		}
	}
}
