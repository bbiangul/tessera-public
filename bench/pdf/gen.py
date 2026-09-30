"""Generates pdf-001's documents from knowledge-001's Markdown, locally:
pandoc (Markdown to HTML and DOCX) and Chrome headless (HTML to PDF). No
network. Also two stress PDFs: the price list as a table, and the
location page in two columns.

  python3 bench/pdf/gen.py
"""
import os, subprocess, tempfile

ROOT = os.path.dirname(os.path.abspath(__file__))
SRC = os.path.join(ROOT, "../knowledge/docs")
CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
CSS = "body{font-family:Arial,sans-serif;font-size:12pt;margin:2cm}h1{font-size:20pt}h2{font-size:15pt}"


def html(body, css=CSS):
    return f"<html><head><meta charset='utf-8'><style>{css}</style></head><body>{body}</body></html>"


def pdf(html_text, out):
    with tempfile.NamedTemporaryFile("w", suffix=".html", delete=False) as f:
        f.write(html_text)
    subprocess.run([CHROME, "--headless=new", "--disable-gpu", "--no-pdf-header-footer", "--print-to-pdf=" + out, "file://" + f.name],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    os.remove(f.name)


if __name__ == "__main__":
    for d in ("pdf", "docx", "stress"):
        os.makedirs(os.path.join(ROOT, d), exist_ok=True)
    for name in sorted(os.listdir(SRC)):
        if not name.endswith(".md"):
            continue
        base = name[:-3]
        md = os.path.join(SRC, name)
        body = subprocess.run(["pandoc", "-f", "markdown", "-t", "html", md], check=True, capture_output=True, text=True).stdout
        pdf(html(body), os.path.join(ROOT, "pdf", base + ".pdf"))
        subprocess.run(["pandoc", md, "-o", os.path.join(ROOT, "docx", base + ".docx")], check=True)
    # Stress: a price table, and two columns.
    table = """<h1>Valores para particulares</h1><table border='1'><tr><th>Procedimento</th><th>Valor</th></tr>
<tr><td>Avaliação inicial (abatida do tratamento feito na clínica em até 60 dias)</td><td>R$ 120</td></tr>
<tr><td>Limpeza (profilaxia)</td><td>R$ 180</td></tr><tr><td>Clareamento em consultório (duas sessões)</td><td>R$ 950</td></tr>
<tr><td>Clareamento caseiro com moldeira</td><td>R$ 650</td></tr><tr><td>Restauração em resina, por dente</td><td>a partir de R$ 220</td></tr></table>
<p>Os valores valem até 31/12/2026.</p>"""
    pdf(html(table), os.path.join(ROOT, "stress", "precos-tabela.pdf"))
    loc = subprocess.run(["pandoc", "-f", "markdown", "-t", "html", os.path.join(SRC, "localizacao.md")], check=True, capture_output=True, text=True).stdout
    pdf(html(loc, CSS + "body{column-count:2;column-gap:1cm}"), os.path.join(ROOT, "stress", "localizacao-duas-colunas.pdf"))
