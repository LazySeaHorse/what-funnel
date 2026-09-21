"""
Unit tests for AI Cascade Tier 1 matching helper (matcher.py).
Verifies clause segmentation, conversational filler stripping,
punctuation normalization, and rapidfuzz pattern matching.
"""

import pytest
from matcher import (
    clean_segment,
    normalize_text,
    segment_inbound,
    match_tier1_patterns,
)


class TestCleanSegment:
    """Tests for clean_segment salutation, filler, and acknowledgment stripping."""

    @pytest.mark.parametrize(
        "greeting",
        [
            "hi",
            "hello",
            "hey",
            "hi there",
            "hey there",
            "hello there",
            "hey guys",
            "hi guys",
            "good morning",
            "good afternoon",
            "good evening",
            "good day",
            "greetings",
        ],
    )
    def test_strip_leading_salutations(self, greeting: str):
        query = f"{greeting}, what are your clinic hours?"
        cleaned = clean_segment(query)
        assert cleaned == "what are your clinic hours"

    @pytest.mark.parametrize(
        "filler",
        [
            "quick question",
            "quick question about your hours",
            "can you tell me",
            "could you tell me",
            "do you know",
            "i was wondering",
            "i was wondering if",
            "just wondering",
            "i have a question",
            "i wanted to ask",
            "i'd like to ask",
            "id like to ask",
            "i would like to know",
            "excuse me",
            "by the way",
            "btw",
            "so",
            "well",
        ],
    )
    def test_strip_conversational_filler(self, filler: str):
        query = f"{filler}, where are you located?"
        cleaned = clean_segment(query)
        assert cleaned == "where are you located"

    @pytest.mark.parametrize(
        "ack",
        [
            "thanks",
            "thank you",
            "thanks so much",
            "thank you so much",
            "many thanks",
            "got it",
            "ok got it",
            "okay got it",
            "understood",
            "that helps",
            "sounds good",
            "awesome",
            "great",
            "cool",
            "okay",
            "ok",
            "perfect",
            "alright",
        ],
    )
    def test_strip_followup_acknowledgments(self, ack: str):
        query = f"{ack}! Do you accept Delta Dental?"
        cleaned = clean_segment(query)
        assert cleaned == "do you accept delta dental"

    def test_strip_chained_fillers(self):
        query = "Hi there! Quick question, can you tell me where is your shop located?"
        # If passed without splitting
        cleaned = clean_segment(query)
        assert cleaned == "where is your shop located"

    def test_only_salutations_or_filler_returns_empty(self):
        assert clean_segment("hello") == ""
        assert clean_segment("hi there!") == ""
        assert clean_segment("quick question") == ""
        assert clean_segment("thanks so much!") == ""
        assert clean_segment("ok got it") == ""
        assert clean_segment("") == ""
        assert clean_segment("   ") == ""
        assert clean_segment(None) == ""

    def test_preserves_legitimate_content_tokens(self):
        assert clean_segment("teeth whitening") == "teeth whitening"
        assert clean_segment("clinic hours") == "clinic hours"
        assert clean_segment("are you hiring dental hygienists?") == "are you hiring dental hygienists"
        assert clean_segment("wellness checkup") == "wellness checkup"
        assert clean_segment("solar panel maintenance") == "solar panel maintenance"


class TestNormalizeText:
    """Tests for normalize_text punctuation, hyphen, and whitespace normalization."""

    def test_normalizes_punctuation(self):
        assert normalize_text("What are your hours?!") == "what are your hours"
        assert normalize_text("Price: $199.00 (tax incl.)") == "price 199 00 tax incl"

    def test_normalizes_hyphens(self):
        assert normalize_text("gluten-free bread") == "gluten free bread"
        assert normalize_text("e-bikes and e-scooters") == "e bikes and e scooters"
        assert normalize_text("take-home whitening kit") == "take home whitening kit"

    def test_normalizes_whitespace(self):
        assert normalize_text("   too   many    spaces   ") == "too many spaces"
        assert normalize_text("line\nbreak\tand\rspaces") == "line break and spaces"

    def test_empty_and_none(self):
        assert normalize_text("") == ""
        assert normalize_text("   ") == ""
        assert normalize_text(None) == ""


class TestSegmentInbound:
    """Tests for segment_inbound clause splitting and non-empty candidate clause extraction."""

    def test_splits_by_punctuation_boundaries(self):
        bubbles = ["Hello! Where are you located? Also, are you open weekends."]
        segments = segment_inbound(bubbles)
        # "Hello" is stripped, leaving the two substantive clauses
        assert segments == ["where are you located", "also, are you open weekends"]

    def test_splits_by_newlines(self):
        bubbles = ["quick question\ndo you take insurance?"]
        segments = segment_inbound(bubbles)
        assert segments == ["do you take insurance"]

    def test_multi_bubble_stripping_and_collection(self):
        bubbles = ["hello", "quick question", "where are you located?"]
        segments = segment_inbound(bubbles)
        assert segments == ["where are you located"]

    def test_multi_bubble_preserves_multiple_meaningful_clauses(self):
        bubbles = [
            "Hi there!",
            "Do you guys do teeth whitening?",
            "And how much does it cost?",
        ]
        segments = segment_inbound(bubbles)
        assert segments == [
            "do you guys do teeth whitening",
            "and how much does it cost",
        ]

    def test_empty_and_whitespace_bubbles(self):
        assert segment_inbound([]) == []
        assert segment_inbound(["", "  ", "!?!"]) == []
        assert segment_inbound([None]) == []


class TestMatchTier1Patterns:
    """Tests for match_tier1_patterns rapidfuzz ratio matching on cleaned segments."""

    @pytest.fixture
    def sample_patterns(self):
        return [
            {
                "id": "dental_whitening",
                "trigger_phrases": [
                    "teeth whitening",
                    "do you do teeth whitening",
                    "how much is teeth whitening",
                ],
                "answer_text": "We offer in-office whitening for $399.",
            },
            {
                "id": "dental_hours",
                "trigger_phrases": [
                    "what are your hours",
                    "clinic hours",
                    "opening hours",
                ],
                "answer_text": "We are open Monday through Friday 8am to 6pm.",
            },
            {
                "id": "bakery_gluten_free",
                "trigger_phrases": [
                    "gluten free options",
                    "do you have gluten free bread",
                    "gluten free",
                ],
                "answer_text": "We bake gluten-free muffins daily.",
            },
        ]

    def test_matches_single_bubble_with_greeting(self, sample_patterns):
        bubbles = ["Hi there, do you guys do teeth whitening?"]
        pattern, score = match_tier1_patterns(sample_patterns, bubbles)
        assert pattern is not None
        assert pattern["id"] == "dental_whitening"
        assert score >= 90.0

    def test_matches_multi_bubble_with_fillers(self, sample_patterns):
        bubbles = ["hello", "quick question", "what are your hours?"]
        pattern, score = match_tier1_patterns(sample_patterns, bubbles)
        assert pattern is not None
        assert pattern["id"] == "dental_hours"
        assert score >= 90.0

    def test_matches_with_hyphen_normalization(self, sample_patterns):
        # Trigger is "gluten free", query has "gluten-free"
        bubbles = ["Do you have gluten-free bread?"]
        pattern, score = match_tier1_patterns(sample_patterns, bubbles)
        assert pattern is not None
        assert pattern["id"] == "bakery_gluten_free"
        assert score >= 90.0

    def test_rejects_unrelated_query(self, sample_patterns):
        bubbles = ["Can I park my helicopter on your roof?"]
        pattern, score = match_tier1_patterns(sample_patterns, bubbles)
        assert pattern is None
        assert score == 0.0

    def test_empty_bubbles_or_patterns(self, sample_patterns):
        assert match_tier1_patterns([], ["teeth whitening"]) == (None, 0.0)
        assert match_tier1_patterns(sample_patterns, []) == (None, 0.0)
        assert match_tier1_patterns(sample_patterns, [""]) == (None, 0.0)

    def test_custom_record_like_objects(self):
        class RecordLike(dict):
            pass

        pat = RecordLike({
            "trigger_phrases": ["emergency dental appointments"],
            "answer_text": "Call our urgent line.",
        })
        bubbles = ["emergency dental appointments"]
        matched, score = match_tier1_patterns([pat], bubbles)
        assert matched is pat
        assert score == 100.0
