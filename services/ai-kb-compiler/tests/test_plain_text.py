from plain_text import normalize_plain_text


def test_normalizes_markdown_and_html_without_losing_content():
    source = "# Pricing\n\n- **Starter** is [available](https://example.test).\n<script>bad()</script>"

    assert normalize_plain_text(source) == "Pricing\n\nStarter is available (https://example.test)."


def test_preserves_ordinary_plain_text_symbols():
    source = "Call +1-555-0100. The price is $5 * 3 = $15."

    assert normalize_plain_text(source) == source


def test_removes_table_rule_strikethrough_and_escapes():
    source = "| Plan | Price |\n| --- | --- |\n| ~~Old~~ New | \\$10 |\n\n---"
    assert normalize_plain_text(source) == "Plan — Price\nOld New — $10"
