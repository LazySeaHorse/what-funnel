"""Groundedness checks: tokenised quantities, number words, locators, named terms."""

import pytest

from grounding import (
    Quantity,
    check_facts,
    extract_locators,
    extract_quantities,
    named_terms,
    unsupported_locators,
    unsupported_named_terms,
    unsupported_quantities,
)

SOURCE = (
    "Opening hours: we are open Monday to Friday 9am to 5pm and Saturday 10:30 am to 2 p.m. Closed Sundays. "
    "A basic tune-up is $80 and takes 3 business days. Refunds within 30 days. Shipping to Canada takes 7 to 10 days. "
    "Call 555-0100 or email help@bike-shop.test, or book at bike-shop.test/service. Delta Dental and Bupa are accepted. "
    "Prices start at $1,500 for the e-bike, with a 15% deposit."
)


def values(answer):
    return [(q.value, q.unit) for q in unsupported_quantities(answer, SOURCE)]


# --- quantities -----------------------------------------------------------------

def test_extract_quantities_units_and_canonical_forms():
    got = extract_quantities("Open 9 am to 5 p.m., $1,500.00 or 15 percent, 30 days, 9:00am, 10:30pm, 007 items, 3rd floor")
    assert Quantity("9", "am") in got and Quantity("5", "pm") in got
    assert Quantity("1500", "$") in got and Quantity("15", "%") in got and Quantity("30", "day") in got
    assert Quantity("9", "am") in got  # 9:00am canonicalises to 9am
    assert Quantity("10:30", "pm") in got
    assert Quantity("7", "") in got  # leading zeros dropped
    assert Quantity("3", "") in got  # ordinal suffix is not a unit


@pytest.mark.parametrize("answer", [
    "We are open 9am to 5pm Monday to Friday.",
    "The tune-up is $80 and takes 3 business days.",
    "Refunds are possible within 30 days.",
    "E-bikes start at $1,500 with a 15% deposit.",
    "Saturday hours are 10:30am to 2pm.",
    "Shipping to Canada takes seven to ten days.",
    "Refunds are available within thirty days.",
    "It takes three business days.",
    "Call 555-0100.",
])
def test_supported_quantities_pass(answer):
    assert values(answer) == []


@pytest.mark.parametrize("answer,expected", [
    ("The tune-up is $8.", ("8", "$")),
    ("We are open 5am.", ("5", "am")),          # source only has 5 p.m.
    ("Prices start at $150.", ("150", "$")),    # 150 is not a substring match for 1,500
    ("E-bikes start at $500.", ("500", "$")),
    ("Refunds within 300 days.", ("300", "day")),
    ("We offer a 5% deposit.", ("5", "%")),     # 5 appears only as 5pm and in 15%
    ("Opens at 10am on Saturday.", ("10", "am")),   # source has 10:30 am, not 10
    ("It takes five days to ship to the US.", ("5", "day")),
    ("The deposit is $15.", ("15", "$")),       # 15 exists only as a percentage, not as a price
    ("Call 555-0199.", ("199", "")),
])
def test_unsupported_quantities_are_detected(answer, expected):
    assert expected in values(answer)


def test_bare_digit_is_not_a_substring_match():
    assert [q.value for q in unsupported_quantities("Room 5 is free", "Rooms 15 and 50 are free")] == ["5"]
    assert unsupported_quantities("Room 15 is free", "Rooms 15 and 50 are free") == []


def test_number_words_and_one_pronoun():
    assert extract_quantities("twenty-five guests")[0] == Quantity("25", "guest")
    assert extract_quantities("a dozen")[0].value == "12"
    assert extract_quantities("one of our staff will help") == []  # "one" is a pronoun here
    assert extract_quantities("within one week")[0] == Quantity("1", "week")
    assert unsupported_quantities("within one week", "returns within 7 days") != []


def test_quantity_without_unit_matches_any_unit_but_a_claimed_price_needs_a_price():
    assert unsupported_quantities("It costs 80", "A tune-up is $80") == []
    assert [q.value for q in unsupported_quantities("It costs $80", "Room 80")] == ["80"]


# --- locators ----------------------------------------------------------------------

def test_extract_locators():
    got = extract_locators("Mail Help@Bike-Shop.test, see https://www.bike-shop.test/service/, or bike-shop.test/testride. e.g. now, 9 a.m. ok")
    assert got == {"help@bike-shop.test", "bike-shop.test/service", "bike-shop.test/testride"}


def test_locators_must_match_whole_tokens():
    assert unsupported_locators("Email help@bike-shop.test", SOURCE) == []
    assert unsupported_locators("Visit https://bike-shop.test/service", SOURCE) == []
    assert unsupported_locators("Email other@bike-shop.test", SOURCE) == ["other@bike-shop.test"]
    assert unsupported_locators("Visit bike-shop.test/refund", SOURCE) == ["bike-shop.test/refund"]
    assert unsupported_locators("Visit bike-shop.test", SOURCE) == ["bike-shop.test"]  # host alone is not the service URL


# --- named terms ----------------------------------------------------------------------

def test_named_terms_ignore_sentence_starts_and_the_pronoun_i():
    assert named_terms("We accept Delta Dental. Please call us. I think Bupa is fine.") == ["Delta", "Dental", "Bupa"]
    assert named_terms("Hello.\nOpen on Monday and Sunday") == ["Monday", "Sunday"]


@pytest.mark.parametrize("answer", [
    "We accept Delta Dental and Bupa.",
    "We are closed on Sundays but open Saturday.",
    "Yes, we are open on Monday to Friday.",
])
def test_supported_named_terms_pass(answer):
    assert unsupported_named_terms(answer, SOURCE) == []


@pytest.mark.parametrize("answer,term", [
    ("Yes, we accept Cigna too.", "Cigna"),
    ("We also have a branch in Dallas.", "Dallas"),
    ("Ask Maria at the front desk.", "Maria"),
    ("We accept Delta Dental and Aetna.", "Aetna"),
])
def test_invented_names_are_detected_even_if_the_customer_mentioned_them(answer, term):
    assert term in unsupported_named_terms(answer, SOURCE)


# --- combined ---------------------------------------------------------------------------

def test_check_facts_reports_the_first_failing_class():
    assert check_facts("We are open 9am to 5pm on Monday.", SOURCE) == (True, "grounded")
    ok, reason = check_facts("The tune-up is $90.", SOURCE)
    assert not ok and reason == "ungrounded_number:90$"
    ok, reason = check_facts("Email x@y.test.", SOURCE)
    assert not ok and reason.startswith("ungrounded_locator:")
    ok, reason = check_facts("We accept Cigna.", SOURCE)
    assert not ok and reason == "ungrounded_term:Cigna"


def test_limits_are_documented_by_a_test():
    """Facts without a number, locator or name are NOT checked: this is why rag defaults to draft-only."""
    assert check_facts("Yes, we offer free refunds on everything and free parking.", SOURCE)[0] is True
