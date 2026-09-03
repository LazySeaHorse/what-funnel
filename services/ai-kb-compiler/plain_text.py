import html
import re
from html.parser import HTMLParser


class _TextExtractor(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.parts: list[str] = []
        self.ignored_depth = 0

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag in {"script", "style"}:
            self.ignored_depth += 1
        elif tag in {"br", "p", "div", "li", "tr"} and self.parts:
            self.parts.append("\n")

    def handle_endtag(self, tag: str) -> None:
        if tag in {"script", "style"} and self.ignored_depth:
            self.ignored_depth -= 1
        elif tag in {"p", "div", "li", "tr"} and self.parts:
            self.parts.append("\n")

    def handle_data(self, data: str) -> None:
        if not self.ignored_depth:
            self.parts.append(data)


def normalize_plain_text(value: str) -> str:
    """Convert model-authored Markdown or HTML into platform-safe plain text."""
    value = html.unescape(value or "").replace("\r\n", "\n").replace("\r", "\n")
    value = re.sub(r"```(?:[\w+-]+)?\s*\n?([\s\S]*?)```", r"\1", value)
    value = re.sub(r"!\[([^\]]*)\]\(([^)]+)\)", r"\1 (\2)", value)
    value = re.sub(r"\[([^\]]+)\]\(([^)]+)\)", r"\1 (\2)", value)
    value = re.sub(r"(?m)^[ \t]{0,3}#{1,6}[ \t]+", "", value)
    value = re.sub(r"(?m)^[ \t]*>[ \t]?", "", value)
    value = re.sub(r"(?m)^[ \t]*(?:[-+*]|\d+[.)])[ \t]+", "", value)
    value = re.sub(r"(?m)^[ \t]*(?:-{3,}|\*{3,}|_{3,})[ \t]*$", "", value)
    value = re.sub(r"(?m)^[ \t]*\|?[ \t]*:?-{3,}:?(?:[ \t]*\|[ \t]*:?-{3,}:?)+[ \t]*\|?[ \t]*(?:\n|$)", "", value)
    value = re.sub(r"(?m)^[ \t]*\|(.+)\|[ \t]*$", lambda match: " — ".join(part.strip() for part in match.group(1).split("|")), value)
    value = re.sub(r"(?<!\\)(\*\*|__)(.+?)\1", r"\2", value)
    value = re.sub(r"~~(.+?)~~", r"\1", value)
    value = re.sub(r"(?<!\\)(\*|_)(.+?)\1", r"\2", value)
    value = re.sub(r"`([^`]+)`", r"\1", value)
    value = re.sub(r"\\([\\`*{}\[\]()#+.!_$>~-])", r"\1", value)

    parser = _TextExtractor()
    parser.feed(value)
    value = "".join(parser.parts)
    value = re.sub(r"[ \t]+\n", "\n", value)
    value = re.sub(r"\n{3,}", "\n\n", value)
    return value.strip()
