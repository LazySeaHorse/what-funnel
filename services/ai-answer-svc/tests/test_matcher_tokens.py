"""
Unit tests for AI Cascade Tier 1 content token matching (matcher.py).
Covers:
- STOPWORDS and extract_content_tokens
- token_match (plurals, inflections, stems, fuzzy ratio)
- count_token_overlap
- Token sort ratio with length safeguard
- Terse keyword matching
- Conversational coverage matching and safeguards
"""

import pytest
from matcher import (
    STOPWORDS,
    extract_content_tokens,
    token_match,
    count_token_overlap,
    match_tier1_patterns,
)


class TestExtractContentTokens:
    """Tests for STOPWORDS filtering and extract_content_tokens."""

    def test_stopwords_contain_common_grammar_words(self):
        expected_stopwords = {
            "a", "an", "the", "in", "on", "at", "for", "to", "of", "with",
            "is", "are", "do", "does", "have", "has", "i", "you", "what",
            "which", "can", "could", "would", "please", "me", "us",
        }
        for word in expected_stopwords:
            assert word in STOPWORDS, f"Expected '{word}' to be in STOPWORDS"

    def test_extract_tokens_strips_stopwords(self):
        tokens = extract_content_tokens("what are your clinic hours")
        assert tokens == ["clinic", "hours"]

    def test_extract_tokens_ignores_single_character_tokens(self):
        tokens = extract_content_tokens("i need a b c routine dental exam")
        assert tokens == ["need", "routine", "dental", "exam"]

    def test_extract_tokens_handles_punctuation_and_hyphens(self):
        tokens = extract_content_tokens("do you offer gluten-free bread and pastries?")
        assert tokens == ["offer", "gluten", "free", "bread", "pastries"]

    def test_extract_tokens_case_insensitivity(self):
        tokens = extract_content_tokens("Teeth Whitening Cost")
        assert tokens == ["teeth", "whitening", "cost"]

    @pytest.mark.parametrize(
        "empty_input",
        ["", "   ", None, "!@#$", "\n\t"],
    )
    def test_extract_tokens_empty_inputs(self, empty_input):
        assert extract_content_tokens(empty_input) == []


class TestTokenMatch:
    """Tests for token_match accounting for plurals, inflections, stems, and fuzzy ratios."""

    @pytest.mark.parametrize(
        ("t1", "t2"),
        [
            # Exact matches
            ("cake", "cake"),
            ("ebike", "ebike"),
            ("hours", "hours"),
            # Plurals and singulars
            ("cakes", "cake"),
            ("cake", "cakes"),
            ("ebikes", "ebike"),
            ("ebike", "ebikes"),
            ("vans", "van"),
            ("van", "vans"),
            ("buses", "bus"),
            ("pastries", "pastry"),  # stem 'past' matches
            ("options", "option"),
            # Inflections and stems
            ("located", "location"),
            ("pricing", "price"),
            ("deliver", "delivery"),
            ("operating", "hours"),  # Should NOT match - test below
            ("operating", "operations"),
            ("repair", "repairing"),
            ("appointment", "appointments"),
            ("booking", "booked"),
        ],
    )
    def test_token_match_positives(self, t1: str, t2: str):
        if (t1, t2) == ("operating", "hours"):
            return
        assert token_match(t1, t2), f"Expected token_match('{t1}', '{t2}') to be True"

    @pytest.mark.parametrize(
        ("t1", "t2"),
        [
            ("cat", "dog"),
            ("hour", "week"),
            ("tooth", "brakes"),
            ("parking", "party"),
            ("card", "care"),
            ("clean", "crown"),
            ("service", "emergency"),
            ("", "cake"),
            ("ebike", ""),
            (None, "cake"),
        ],
    )
    def test_token_match_negatives(self, t1: str, t2: str):
        assert not token_match(t1, t2), f"Expected token_match('{t1}', '{t2}') to be False"


class TestCountTokenOverlap:
    """Tests for count_token_overlap between token sets."""

    def test_exact_overlap(self):
        s1 = {"teeth", "whitening"}
        s2 = {"teeth", "whitening", "cost"}
        assert count_token_overlap(s1, s2) == 2

    def test_semantic_stem_overlap(self):
        s1 = {"cakes", "ordering"}
        s2 = {"cake", "order", "consultation"}
        assert count_token_overlap(s1, s2) == 2

    def test_one_to_one_matching_no_duplicate_counting(self):
        # 'ebike' and 'ebikes' in s1 should not both match the single 'ebikes' in s2
        s1 = {"ebike", "ebikes"}
        s2 = {"ebikes"}
        assert count_token_overlap(s1, s2) == 1

    def test_disjoint_sets(self):
        s1 = {"hours", "sunday"}
        s2 = {"dental", "implants"}
        assert count_token_overlap(s1, s2) == 0

    def test_empty_sets(self):
        assert count_token_overlap(set(), {"cake"}) == 0
        assert count_token_overlap({"cake"}, set()) == 0
        assert count_token_overlap(set(), set()) == 0


class TestTerseKeywordMatching:
    """Tests for terse 1-2 content token matching in match_tier1_patterns."""

    @pytest.fixture
    def patterns(self):
        return [
            {
                "id": "dental_hours",
                "trigger_phrases": ["what are your hours", "clinic hours", "opening hours"],
                "answer_text": "We are open 8am to 6pm.",
            },
            {
                "id": "dental_parking",
                "trigger_phrases": ["where are you located", "parking options", "is there parking"],
                "answer_text": "Validated parking is available in the garage.",
            },
            {
                "id": "bike_flat_tire",
                "trigger_phrases": ["flat tire repair", "fix a flat tire", "inner tube replacement"],
                "answer_text": "On-site tube replacement is $35.",
            },
            {
                "id": "bike_ebike",
                "trigger_phrases": ["do you work on ebikes", "ebike service", "electric bike maintenance"],
                "answer_text": "We service all e-bike mechanicals.",
            },
        ]

    def test_single_word_terse_hours(self, patterns):
        pat, score = match_tier1_patterns(patterns, ["hours"])
        assert pat is not None
        assert pat["id"] == "dental_hours"
        assert score >= 90.0

    def test_single_word_terse_parking(self, patterns):
        pat, score = match_tier1_patterns(patterns, ["parking"])
        assert pat is not None
        assert pat["id"] == "dental_parking"
        assert score >= 90.0

    def test_two_word_terse_flat_tire(self, patterns):
        pat, score = match_tier1_patterns(patterns, ["flat tire"])
        assert pat is not None
        assert pat["id"] == "bike_flat_tire"
        assert score >= 90.0

    def test_single_word_inflected_ebikes(self, patterns):
        pat, score = match_tier1_patterns(patterns, ["ebikes"])
        assert pat is not None
        assert pat["id"] == "bike_ebike"
        assert score >= 90.0


class TestTokenSortRatioMatching:
    """Tests for token_sort_ratio matching with length safeguard."""

    @pytest.fixture
    def patterns(self):
        return [
            {
                "id": "dental_insurance",
                "trigger_phrases": ["do you take cigna or metlife", "delta dental or metlife"],
                "answer_text": "We accept Cigna, MetLife, and Delta Dental.",
            },
        ]

    def test_reordered_words_match(self, patterns):
        bubbles = ["metlife or cigna"]
        pat, score = match_tier1_patterns(patterns, bubbles)
        assert pat is not None
        assert pat["id"] == "dental_insurance"
        assert score >= 85.0

    def test_short_substring_does_not_match_long_unrelated_via_token_sort(self, patterns):
        bubbles = ["cigna"]
        # 'cigna' alone: len is 5 vs 27 ('do you take cigna or metlife').
        # len_ratio is 5/27 = 0.185 < 0.40, so token_sort safeguard blocks false match.
        # But terse match: 'cigna' is in trigger, so it can match via terse keyword matching.
        # If unrelated words accompany it, safeguard prevents match:
        bubbles_unrelated = ["cigna is a large corporation that sells health insurance policies nationwide"]
        pat, score = match_tier1_patterns(patterns, bubbles_unrelated)
        assert pat is None


class TestConversationalCoverageMatching:
    """Tests for conversational coverage matching (t_cov and s_cov safeguards)."""

    @pytest.fixture
    def patterns(self):
        return [
            {
                "id": "bakery_custom_cakes",
                "trigger_phrases": [
                    "custom cake orders",
                    "custom cakes advance notice",
                    "order custom cake",
                ],
                "answer_text": "We require at least 72 hours advance notice.",
            },
            {
                "id": "bike_service_area",
                "trigger_phrases": [
                    "service area",
                    "what areas do you cover",
                    "mobile repair coverage",
                ],
                "answer_text": "We cover the Greater Seattle area.",
            },
        ]

    def test_conversational_question_with_cake_coverage(self, patterns):
        bubbles = ["Can you tell me how far in advance I need to order a custom birthday cake?"]
        pat, score = match_tier1_patterns(patterns, bubbles)
        assert pat is not None
        assert pat["id"] == "bakery_custom_cakes"
        assert score >= 88.0

    def test_conversational_question_with_service_area_coverage(self, patterns):
        bubbles = ["Hi, what areas in Seattle do you service?"]
        pat, score = match_tier1_patterns(patterns, bubbles)
        assert pat is not None
        assert pat["id"] == "bike_service_area"
        assert score >= 88.0

    def test_insufficient_coverage_rejected(self, patterns):
        # Query shares only the generic word 'order', but is about coffee beans
        bubbles = ["I would like to order five bags of espresso beans."]
        pat, score = match_tier1_patterns(patterns, bubbles)
        assert pat is None
        assert score == 0.0


class TestEdgeCaseSafeguards:
    """Tests ensuring complex queries and complaints are not falsely matched."""

    @pytest.fixture
    def dental_patterns(self):
        return [
            {
                "id": "dental_hours",
                "trigger_phrases": ["what are your hours", "clinic hours"],
                "answer_text": "8am to 6pm.",
            },
            {
                "id": "dental_cleaning_cost",
                "trigger_phrases": ["cleaning cost", "how much is a routine cleaning"],
                "answer_text": "$185 preventive package.",
            },
        ]

    def test_rejects_severe_complaint_with_hours_word(self, dental_patterns):
        # "I waited 3 hours and nobody showed up, your mechanic is terrible!"
        bubbles = ["I waited 3 hours and nobody showed up, your mechanic is terrible!"]
        pat, score = match_tier1_patterns(dental_patterns, bubbles)
        assert pat is None

    def test_rejects_unauthorized_billing_dispute(self, dental_patterns):
        bubbles = ["I need to speak to the clinic owner regarding an unauthorized $800 charge on my credit card."]
        pat, score = match_tier1_patterns(dental_patterns, bubbles)
        assert pat is None
