import csv
import io
import re
import zipfile
import xml.etree.ElementTree as ET
from typing import List, Tuple

from docx import Document
from fastapi import FastAPI, File, Form, UploadFile
from pypdf import PdfReader

app = FastAPI(title="parser-service")


def chunk_text(text: str, chunk_size: int = 2000) -> List[str]:
    text = re.sub(r"\s+", " ", text).strip()
    if not text:
        return []
    chunks: List[str] = []
    for i in range(0, len(text), chunk_size):
        chunks.append(text[i : i + chunk_size].strip())
    return [c for c in chunks if c]


def parse_csv(data: bytes) -> Tuple[List[str], int]:
    decoded = data.decode("utf-8", errors="replace")
    reader = csv.reader(io.StringIO(decoded))
    records = list(reader)
    if not records:
        return [], 0
    headers = records[0]
    rows = records[1:]
    chunk_rows = 50
    chunks: List[str] = []
    for i in range(0, len(rows), chunk_rows):
        section = rows[i : i + chunk_rows]
        lines = [f"CSV Data (columns: {', '.join(headers)})", ""]
        for row in section:
            for idx, cell in enumerate(row):
                if idx < len(headers):
                    lines.append(f"{headers[idx]}: {cell}")
            lines.append("---")
        chunks.append("\n".join(lines).strip())
    return chunks, len(rows)


def parse_txt(data: bytes) -> List[str]:
    decoded = data.decode("utf-8", errors="replace")
    return chunk_text(decoded)


def parse_docx(data: bytes) -> List[str]:
    # Try high-level parser first.
    try:
        with io.BytesIO(data) as b:
            doc = Document(b)
        text_parts = [p.text for p in doc.paragraphs if p.text and p.text.strip()]
        text = "\n".join(text_parts)
        chunks = chunk_text(text)
        if chunks:
            return chunks
    except Exception:
        pass

    # Fallback: parse raw OOXML from word/document.xml.
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as z:
            raw = z.read("word/document.xml")
        root = ET.fromstring(raw)
        text_nodes = []
        for elem in root.iter():
            if elem.tag.endswith("}t") and elem.text:
                text_nodes.append(elem.text.strip())
        return chunk_text(" ".join([t for t in text_nodes if t]))
    except Exception:
        return []


def parse_pdf(data: bytes) -> List[str]:
    with io.BytesIO(data) as b:
        reader = PdfReader(b)
        pages = []
        for page in reader.pages:
            txt = page.extract_text() or ""
            if txt.strip():
                pages.append(txt)
    return chunk_text("\n".join(pages))


def detect_from_name(filename: str) -> str:
    lower = filename.lower()
    if lower.endswith(".csv"):
        return "csv"
    if lower.endswith(".pdf"):
        return "pdf"
    if lower.endswith(".docx") or lower.endswith(".doc"):
        return "docx"
    return "txt"


def binary_text_fallback(data: bytes) -> List[str]:
    printable = "".join(chr(b) if 32 <= b < 127 else " " for b in data)
    return chunk_text(printable)


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "service": "parser-service"}


@app.post("/parse")
async def parse(
    file: UploadFile = File(...),
    file_type: str = Form(default=""),
    mime_type: str = Form(default=""),
) -> dict:
    data = await file.read()
    effective_type = (file_type or detect_from_name(file.filename or "")).lower()

    chunks: List[str] = []
    row_count = 0
    try:
        if effective_type == "csv":
            chunks, row_count = parse_csv(data)
        elif effective_type == "pdf":
            chunks = parse_pdf(data)
        elif effective_type == "docx":
            chunks = parse_docx(data)
        else:
            chunks = parse_txt(data)
    except Exception:
        chunks = []

    if not chunks:
        # For malformed binaries or unsupported docs, still return searchable text.
        chunks = binary_text_fallback(data)
    if not chunks:
        chunks = [f"File uploaded: {file.filename or 'unknown'} ({len(data)} bytes)"]

    return {
        "chunks": chunks,
        "row_count": row_count,
        "file_type": effective_type,
        "mime_type": mime_type,
    }
