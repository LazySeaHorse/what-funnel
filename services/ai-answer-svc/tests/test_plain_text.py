from plain_text import normalize_plain_text


def test_normalizes_model_formatting_to_plain_text():
    assert normalize_plain_text("**Yes.** See [pricing](https://example.test).") == "Yes. See pricing (https://example.test)."


def test_removes_active_html_content():
    assert normalize_plain_text("Hello<script>alert(1)</script> world") == "Hello world"


def test_removes_table_rule_strikethrough_and_escapes():
    source = "| Plan | Price |\n| --- | --- |\n| ~~Old~~ New | \\$10 |\n\n---"
    assert normalize_plain_text(source) == "Plan — Price\nOld New — $10"
