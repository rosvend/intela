# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///
# Converts the REDES source documents into citable markdown under docs/reglamentos.
# Layer 1 of the knowledge base: verbatim text, split by top level section.
# Requires pdftotext (poppler-utils: apt install poppler-utils / brew install poppler).
# Run: uv run --script src/scripts/convert_reglamentos.py

import re
import shutil
import subprocess
import unicodedata
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SOURCE_DIR = ROOT / "docs" / "reglamentos" / "fuente"
OUTPUT_DIR = ROOT / "docs" / "reglamentos"

DOCUMENTS = {
    "20260527-Version-IX-Reglamento-de-Distribucion-Aprobada-por-el-Consejo.pdf": {
        "slug": "distribucion-v9",
        "titulo": "Reglamento de Distribucion",
        "version": "IX",
        "aprobado": "2026-05-27",
        "secciones": [
            "Presentación",
            "Objetivo del Reglamento",
            "Definiciones",
            "Principios",
            "Reparto",
            "Derechos Objeto de Reparto",
            "Titulares del Derecho de Recaudo",
            "Modalidad del acto de comunicación pública y de tipo de usuario",
            "Metodología para la distribución",
            "Distribución de rendimiento financiero",
            "Determinación de las asignaciones a distribuir",
            "Fechas de Distribución",
            "Sistema de Distribución",
            "Reserva para corrección de errores técnicos",
            "Prescripciones",
            "Auditoría (Inspección y vigilancia)",
            "Disposiciones finales",
        ],
        "notas_extraccion": [
            "La ecuacion de la seccion 9.7 (plataformas OTT) es un objeto de ecuacion de Word "
            "y pdftotext no la extrae. Solo quedan las definiciones de sus variables. "
            "La ecuacion transcrita desde el PDF esta en docs/dominio/formulas.md.",
            "Los diagramas de flujo de la seccion 13.5 se extraen como texto plano sin la "
            "estructura del diagrama.",
        ],
    },
    "Reglamento-de-tarifas-Version-VI.pdf": {
        "slug": "tarifas-v6",
        "titulo": "Reglamento de Tarifas",
        "version": "VI",
        "aprobado": "2026-06-30",
        "secciones": [
            "Objeto del Reglamento",
            "Definiciones",
            "Tarifas según la categoría de Usuario",
            "Tabla resumen tarifas propuestas",
            "Disposiciones finales",
        ],
    },
    "Reglamento-de-socios.pdf": {
        "slug": "socios",
        "titulo": "Reglamento de Socios",
        "version": "V5",
        "aprobado": "sin fecha en el documento",
        "secciones": [
            "Objetivo del reglamento",
            "Ámbito de aplicación",
            "Órganos competentes",
            "Tipos de Afiliados",
            "Procedimiento y requisitos para ser Afiliados",
            "Disposiciones Finales",
        ],
    },
    "1.-Reglamento-Anticipo-de-Afiliados.pdf": {
        "slug": "anticipos",
        "titulo": "Reglamento de Anticipos a Afiliados",
        "version": "V7",
        "aprobado": "sin fecha en el documento",
        "secciones": [
            "Objetivo del Reglamento",
            "Anticipos de Derechos de Autor",
            "Solicitud de Anticipo por Derechos de Autor",
            "Disposiciones finales",
        ],
    },
}

FOOTER_PATTERNS = (
    r"^RED COLOMBIANA DE ESCRITORES AUDIOVISUALES.*",
    r"^DISTRIBUCI[OÓ]N IX$",
    r"^REDES SGC .{0,3} REGLAMENTO DE TARIFAS.*",
    r"^REDES .{0,3} REGLAMENTO DE ANTICIPOS.*",
    r"^REDES [-–] Reglamento de.*",
    r"^REDES SGC- Reglamento de.*",
    r"^Aprobado por el Consejo Directivo el .*",
    r"^Tabla de contenido$",
    r"^\d{1,3}$",
)

HEADING = re.compile(r"^(\d{1,2})[.)]?\s+(\S.*)$")
TOC_ROW = re.compile(r"^\d{1,2}[.)]?\s+.*\s{2,}\d{1,3}$")


def extract_text(pdf_path):
    if shutil.which("pdftotext") is None:
        raise SystemExit("pdftotext not found. Install poppler-utils.")
    result = subprocess.run(
        ["pdftotext", "-layout", str(pdf_path), "-"],
        capture_output=True,
        text=True,
        check=True,
    )
    return result.stdout


def is_footer(line):
    return any(re.match(pattern, line) for pattern in FOOTER_PATTERNS)


def is_toc_entry(line):
    return "...." in line


def is_tabular(line):
    return re.search(r"\S {3,}\S", line) is not None


def clean_lines(text):
    lines = []
    page = 1
    for raw in text.split("\n"):
        if "\f" in raw:
            page += raw.count("\f")
            raw = raw.replace("\f", "")
        stripped = raw.strip()
        if not stripped or is_footer(stripped) or is_toc_entry(stripped) or TOC_ROW.match(stripped):
            lines.append(("", page, 0))
            continue
        indent = len(raw) - len(raw.lstrip())
        lines.append((raw.rstrip() if is_tabular(raw) else stripped, page, indent))
    return lines


def slugify(value):
    text = unicodedata.normalize("NFKD", value)
    text = "".join(char for char in text if not unicodedata.combining(char))
    text = re.sub(r"[^a-z0-9]+", "-", text.lower())
    return text.strip("-")[:50]


def matches_expected_title(text, expected_title):
    return slugify(text).startswith(slugify(expected_title)[:24])


def split_sections(lines, expected_titles):
    sections = [{"number": 0, "title": "Presentacion", "lines": [], "pages": set()}]
    index = 0
    for content, page, indent in lines:
        match = HEADING.match(content) if indent == 0 else None
        if match and index < len(expected_titles) and int(match.group(1)) == index + 1:
            if matches_expected_title(match.group(2), expected_titles[index]):
                index += 1
                sections.append(
                    {"number": index, "title": expected_titles[index - 1], "lines": [], "pages": set()}
                )
                continue
        sections[-1]["lines"].append(content)
        if content:
            sections[-1]["pages"].add(page)
    if index != len(expected_titles):
        print(f"  WARNING: found {index} of {len(expected_titles)} expected sections")
    return sections


def collapse_blanks(lines):
    output = []
    for line in lines:
        if not line and (not output or not output[-1]):
            continue
        output.append(line)
    while output and not output[-1]:
        output.pop()
    return output


def page_range(pages):
    if not pages:
        return "?"
    low, high = min(pages), max(pages)
    return str(low) if low == high else f"{low}-{high}"


def render_section(meta, section, source_name):
    body = collapse_blanks(section["lines"])
    heading = f"{section['number']}. {section['title']}" if section["number"] else section["title"]
    front = [
        "---",
        f"reglamento: {meta['titulo']}",
        f"version: {meta['version']}",
        f"aprobado: {meta['aprobado']}",
        f"seccion: {heading}",
        f"paginas_pdf: {page_range(section['pages'])}",
        f"fuente: docs/reglamentos/fuente/{source_name}",
        "generado_por: src/scripts/convert_reglamentos.py",
        "---",
        "",
        f"# {heading}",
        "",
    ]
    return "\n".join(front + body) + "\n"


def write_index(target_dir, meta, sections, source_name):
    rows = [
        "---",
        f"reglamento: {meta['titulo']}",
        f"version: {meta['version']}",
        f"aprobado: {meta['aprobado']}",
        f"fuente: docs/reglamentos/fuente/{source_name}",
        "---",
        "",
        f"# {meta['titulo']} (Version {meta['version']})",
        "",
        f"Aprobado: {meta['aprobado']}",
        f"PDF original: `docs/reglamentos/fuente/{source_name}`",
        "",
        "| Seccion | Archivo | Paginas PDF |",
        "| --- | --- | --- |",
    ]
    for section, filename in sections:
        heading = f"{section['number']}. {section['title']}" if section["number"] else section["title"]
        rows.append(f"| {heading} | [{filename}]({filename}) | {page_range(section['pages'])} |")

    notas = meta.get("notas_extraccion", [])
    if notas:
        rows += ["", "## Limitaciones de la extraccion", ""]
        rows += [f"- {nota}" for nota in notas]

    (target_dir / "00-indice.md").write_text("\n".join(rows) + "\n", encoding="utf-8")


def convert(source_name, meta):
    pdf_path = SOURCE_DIR / source_name
    target_dir = OUTPUT_DIR / meta["slug"]
    target_dir.mkdir(parents=True, exist_ok=True)
    for existing in target_dir.glob("*.md"):
        existing.unlink()

    sections = split_sections(clean_lines(extract_text(pdf_path)), meta["secciones"])
    written = []
    for section in sections:
        if not collapse_blanks(section["lines"]):
            continue
        filename = f"{section['number']:02d}-{slugify(section['title'])}.md"
        (target_dir / filename).write_text(render_section(meta, section, source_name), encoding="utf-8")
        written.append((section, filename))

    write_index(target_dir, meta, written, source_name)
    print(f"{meta['slug']}: {len(written)} sections")
    for section, filename in written:
        heading = f"{section['number']}. {section['title']}" if section["number"] else section["title"]
        print(f"  {filename:<45} {heading}")


def main():
    for source_name, meta in DOCUMENTS.items():
        convert(source_name, meta)


if __name__ == "__main__":
    main()
