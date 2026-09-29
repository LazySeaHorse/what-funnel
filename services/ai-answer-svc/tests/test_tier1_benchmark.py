"""
Table-driven benchmark of the real Tier 1 matcher (matcher.match_tier1_patterns)
against a dataset of customer inquiries across 3 small businesses.

Every case is exercised against the production function itself; there are no local
re-implementations of the matching algorithm in this file.
"""

from dataclasses import dataclass

import pytest

from matcher import match_tier1_patterns

# ==============================================================================
# 1. Realistic Small Business Definitions & Knowledge Base Patterns
# ==============================================================================

BUSINESSES = {
    "apex_dental": {
        "name": "Apex Dental Studio",
        "industry": "Dental Clinic",
        "patterns": [
            {
                "id": "dental_teeth_whitening",
                "canonical_question": "Do you offer teeth whitening services?",
                "trigger_phrases": [
                    "teeth whitening",
                    "do you do teeth whitening",
                    "how much is teeth whitening",
                    "teeth whitening cost",
                    "do you offer teeth whitening",
                    "professional teeth whitening"
                ],
                "answer_text": "Yes! We offer in-office Zoom teeth whitening for $399 and custom take-home whitening trays for $199. Both options include a preliminary shade assessment."
            },
            {
                "id": "dental_insurance",
                "canonical_question": "What dental insurance plans do you accept?",
                "trigger_phrases": [
                    "what insurance do you take",
                    "do you accept insurance",
                    "insurance plans accepted",
                    "do you take delta dental",
                    "accepted insurance",
                    "which insurance do you accept",
                    "do you take cigna or metlife",
                    "cigna or metlife"
                ],
                "answer_text": "We are in-network with Delta Dental, Cigna, MetLife, and Guardian. For out-of-network plans, we can submit claims on your behalf."
            },
            {
                "id": "dental_hours",
                "canonical_question": "What are your clinic hours?",
                "trigger_phrases": [
                    "what are your hours",
                    "clinic hours",
                    "opening hours",
                    "are you open on weekends",
                    "when are you open",
                    "business hours",
                    "what time do you open"
                ],
                "answer_text": "Apex Dental Studio is open Monday through Thursday from 8:00 AM to 6:00 PM, Friday from 8:00 AM to 3:00 PM, and closed Saturday and Sunday."
            },
            {
                "id": "dental_parking_location",
                "canonical_question": "Where are you located and is there parking?",
                "trigger_phrases": [
                    "where are you located",
                    "clinic address",
                    "is there parking",
                    "parking options",
                    "where is your office",
                    "directions and parking"
                ],
                "answer_text": "We are located at 450 Sutter St, Suite 1200, San Francisco. Validated patient parking is available in the underground garage accessed via Bush Street."
            },
            {
                "id": "dental_cleaning_cost",
                "canonical_question": "How much does a routine cleaning and exam cost without insurance?",
                "trigger_phrases": [
                    "cleaning cost",
                    "how much is a routine cleaning",
                    "cleaning and exam price",
                    "cash price for cleaning",
                    "routine checkup cost",
                    "dental exam cost"
                ],
                "answer_text": "Our new patient preventive package is $185, which includes a comprehensive dental exam, full-mouth digital X-rays, and standard cleaning."
            },
            {
                "id": "dental_emergency_policy",
                "canonical_question": "Do you offer emergency dental appointments?",
                "trigger_phrases": [
                    "emergency dental appointments",
                    "do you take emergencies",
                    "emergency dentist",
                    "same day emergency",
                    "urgent dental care",
                    "emergency appointments"
                ],
                "answer_text": "Yes, we reserve dedicated slots daily for urgent dental emergencies like severe tooth pain or chipped teeth. Please call our urgent line at (415) 555-0199 for immediate triage."
            }
        ]
    },
    "bella_roma": {
        "name": "Bella Roma Artisanal Bakery",
        "industry": "Bakery / Cafe",
        "patterns": [
            {
                "id": "bakery_hours",
                "canonical_question": "What are your bakery operating hours?",
                "trigger_phrases": [
                    "what are your hours",
                    "bakery hours",
                    "opening hours",
                    "when do you open",
                    "are you open sunday",
                    "operating hours",
                    "what time do you open"
                ],
                "answer_text": "Bella Roma is open Tuesday through Saturday from 7:00 AM to 5:00 PM, and Sunday from 8:00 AM to 2:00 PM. We are closed on Mondays."
            },
            {
                "id": "bakery_gluten_free",
                "canonical_question": "Do you offer gluten-free bread and pastries?",
                "trigger_phrases": [
                    "gluten free options",
                    "do you have gluten free bread",
                    "gluten free pastries",
                    "any gluten free items",
                    "celiac friendly",
                    "gluten free"
                ],
                "answer_text": "We bake fresh gluten-free almond flour muffins and flourless chocolate torte daily. However, because our kitchen handles wheat flour, items are not recommended for severe celiac disease."
            },
            {
                "id": "bakery_custom_cakes",
                "canonical_question": "How far in advance do I need to order a custom cake?",
                "trigger_phrases": [
                    "custom cake orders",
                    "custom cakes advance notice",
                    "order custom cake",
                    "birthday cake order",
                    "how much notice for custom cake",
                    "wedding cake consultation"
                ],
                "answer_text": "We require at least 72 hours advance notice for custom birthday and celebration cakes. Tiered wedding cakes require a consultation and at least 3 weeks notice."
            },
            {
                "id": "bakery_delivery",
                "canonical_question": "Do you deliver or offer catering delivery?",
                "trigger_phrases": [
                    "do you deliver",
                    "delivery options",
                    "catering delivery",
                    "door dash or uber eats",
                    "bakery delivery",
                    "local delivery"
                ],
                "answer_text": "Local delivery is available within 5 miles for catering orders over $75 placed 24 hours in advance. Individual orders can be ordered on DoorDash and UberEats."
            },
            {
                "id": "bakery_vegan",
                "canonical_question": "Do you have vegan pastry options?",
                "trigger_phrases": [
                    "vegan options",
                    "do you have vegan pastries",
                    "vegan croissants",
                    "plant based pastries",
                    "dairy free pastries",
                    "vegan treats"
                ],
                "answer_text": "Yes! We offer vegan cinnamon rolls, plant-based sourdough bagels, oat milk for espresso, and a rotating vegan fruit tart every morning."
            },
            {
                "id": "bakery_location",
                "canonical_question": "Where is Bella Roma Bakery located?",
                "trigger_phrases": [
                    "where are you located",
                    "bakery address",
                    "where is your shop",
                    "directions to bakery",
                    "street address",
                    "bakery location"
                ],
                "answer_text": "We are located at 742 Columbus Ave in North Beach, San Francisco, right next to Washington Square Park."
            }
        ]
    },
    "gearshift_bike": {
        "name": "GearShift Mobile Bicycle Repair",
        "industry": "Mobile Bike Mechanic",
        "patterns": [
            {
                "id": "bike_service_area",
                "canonical_question": "What areas do you service?",
                "trigger_phrases": [
                    "service area",
                    "what areas do you cover",
                    "do you service my area",
                    "where do you travel to",
                    "mobile repair coverage",
                    "service locations",
                    "do you service bellevue or redmond",
                    "bellevue bike repair"
                ],
                "answer_text": "GearShift serves the entire Greater Seattle metro area including Seattle, Bellevue, Kirkland, Redmond, and Renton. There is no travel fee within 15 miles of downtown Seattle."
            },
            {
                "id": "bike_tuneup_cost",
                "canonical_question": "How much does a basic bicycle tune-up cost?",
                "trigger_phrases": [
                    "tune up cost",
                    "how much is a tune up",
                    "basic tune up pricing",
                    "pricing for tune up",
                    "bike service rates",
                    "tune up packages"
                ],
                "answer_text": "Our Standard Mobile Tune-Up is $95 and includes brake & derailleur adjustment, drivetrain degrease/lube, wheel truing, bolt torquing, and safety inspection."
            },
            {
                "id": "bike_flat_tire",
                "canonical_question": "Can you fix a flat tire on-site today?",
                "trigger_phrases": [
                    "flat tire repair",
                    "fix a flat tire",
                    "can you fix flat tire",
                    "how much for flat tire",
                    "inner tube replacement",
                    "puncture repair"
                ],
                "answer_text": "Yes! On-site inner tube replacement is $35 (tube included) or $20 when added to any tune-up. Subject to today's dispatch availability."
            },
            {
                "id": "bike_ebike_service",
                "canonical_question": "Do you service electric bikes (e-bikes)?",
                "trigger_phrases": [
                    "do you work on ebikes",
                    "ebike service",
                    "electric bike maintenance",
                    "do you fix electric bikes",
                    "ebike tune up",
                    "ebike motor repair"
                ],
                "answer_text": "We service mechanical components (brakes, gears, tires, chains) on all e-bikes, and Bosch/Shimano motor systems. We do not service generic direct-drive hub batteries."
            },
            {
                "id": "bike_booking_process",
                "canonical_question": "How do I book a mobile bike repair appointment?",
                "trigger_phrases": [
                    "how do i book",
                    "book an appointment",
                    "schedule a repair",
                    "how to schedule",
                    "booking process",
                    "how does mobile repair work"
                ],
                "answer_text": "You can book online at gearshiftbike.com/book by choosing your service and selecting a 1-hour arrival window. Our van arrives fully equipped to fix your bike right at your home or office."
            },
            {
                "id": "bike_hours",
                "canonical_question": "What days and hours do your mobile vans operate?",
                "trigger_phrases": [
                    "operating hours",
                    "what are your hours",
                    "working hours",
                    "are you open on weekends",
                    "van operating hours",
                    "service hours"
                ],
                "answer_text": "Our mobile vans operate 7 days a week: Monday through Friday 7:30 AM to 7:00 PM, and Saturday & Sunday 8:00 AM to 5:00 PM."
            }
        ]
    }
}

# ==============================================================================
# 2. Benchmark Dataset: 66 Real-World Customer Inbound Queries
# ==============================================================================

@dataclass
class BenchmarkCase:
    id: str
    business: str
    category: str  # "terse_keyword", "first_question", "follow_up", "multi_bubble", "edge_case_negative"
    bubbles: list[str]
    expected_match: bool
    expected_pattern_id: str | None
    description: str

BENCHMARK_CASES: list[BenchmarkCase] = [
    # --------------------------------------------------------------------------
    # Category 1: Terse / Keyword Inputs (12 queries)
    # Expected: Mundane FAQ match
    # --------------------------------------------------------------------------
    BenchmarkCase("T01", "apex_dental", "terse_keyword", ["hours"], True, "dental_hours", "Single word 'hours'"),
    BenchmarkCase("T02", "apex_dental", "terse_keyword", ["parking"], True, "dental_parking_location", "Single word 'parking'"),
    BenchmarkCase("T03", "apex_dental", "terse_keyword", ["insurance"], True, "dental_insurance", "Single word 'insurance'"),
    BenchmarkCase("T04", "apex_dental", "terse_keyword", ["teeth whitening"], True, "dental_teeth_whitening", "Verbatim trigger 'teeth whitening'"),
    BenchmarkCase("T05", "bella_roma", "terse_keyword", ["opening hours"], True, "bakery_hours", "Direct phrase 'opening hours'"),
    BenchmarkCase("T06", "bella_roma", "terse_keyword", ["gluten free"], True, "bakery_gluten_free", "Verbatim trigger 'gluten free'"),
    BenchmarkCase("T07", "bella_roma", "terse_keyword", ["vegan options"], True, "bakery_vegan", "Verbatim trigger 'vegan options'"),
    BenchmarkCase("T08", "bella_roma", "terse_keyword", ["delivery"], True, "bakery_delivery", "Single word 'delivery'"),
    BenchmarkCase("T09", "gearshift_bike", "terse_keyword", ["pricing"], True, "bike_tuneup_cost", "Single word 'pricing'"),
    BenchmarkCase("T10", "gearshift_bike", "terse_keyword", ["service area"], True, "bike_service_area", "Verbatim trigger 'service area'"),
    BenchmarkCase("T11", "gearshift_bike", "terse_keyword", ["flat tire"], True, "bike_flat_tire", "Two-word keyword 'flat tire'"),
    BenchmarkCase("T12", "gearshift_bike", "terse_keyword", ["ebikes"], True, "bike_ebike_service", "Keyword 'ebikes'"),

    # --------------------------------------------------------------------------
    # Category 2: Conversational First Questions (15 queries)
    # Expected: Mundane FAQ match
    # --------------------------------------------------------------------------
    BenchmarkCase("F01", "apex_dental", "first_question", ["Hi there, do you guys do teeth whitening?"], True, "dental_teeth_whitening", "Conversational greeting + whitening"),
    BenchmarkCase("F02", "bella_roma", "first_question", ["What are your hours on Sunday?"], True, "bakery_hours", "Specific day inquiry for hours"),
    BenchmarkCase("F03", "apex_dental", "first_question", ["Good morning, do you take Delta Dental insurance?"], True, "dental_insurance", "Polite greeting + Delta Dental"),
    BenchmarkCase("F04", "apex_dental", "first_question", ["Hello! How much is a routine cleaning and exam?"], True, "dental_cleaning_cost", "Greeting + cleaning cost inquiry"),
    BenchmarkCase("F05", "bella_roma", "first_question", ["Hey there! Where is your bakery located?"], True, "bakery_location", "Greeting + bakery address"),
    BenchmarkCase("F06", "bella_roma", "first_question", ["Hi, do you offer any gluten-free bread?"], True, "bakery_gluten_free", "Conversational gluten free inquiry"),
    BenchmarkCase("F07", "bella_roma", "first_question", ["Can you tell me how far in advance I need to order a custom birthday cake?"], True, "bakery_custom_cakes", "Polite question on custom cake lead time"),
    BenchmarkCase("F08", "bella_roma", "first_question", ["Do you deliver catering to the Financial District?"], True, "bakery_delivery", "Catering delivery inquiry"),
    BenchmarkCase("F09", "bella_roma", "first_question", ["Hello, do you guys have vegan pastries available?"], True, "bakery_vegan", "Vegan pastries availability inquiry"),
    BenchmarkCase("F10", "gearshift_bike", "first_question", ["Hi, what areas in Seattle do you service?"], True, "bike_service_area", "Service area coverage question"),
    BenchmarkCase("F11", "gearshift_bike", "first_question", ["Hey, how much does a basic tune-up cost?"], True, "bike_tuneup_cost", "Tune-up pricing inquiry"),
    BenchmarkCase("F12", "gearshift_bike", "first_question", ["Can you guys fix a flat bike tire at my office today?"], True, "bike_flat_tire", "Same-day flat tire repair inquiry"),
    BenchmarkCase("F13", "gearshift_bike", "first_question", ["Do you guys work on electric bikes?"], True, "bike_ebike_service", "Ebike capability inquiry"),
    BenchmarkCase("F14", "gearshift_bike", "first_question", ["How do I book a mobile bike repair appointment?"], True, "bike_booking_process", "Booking process inquiry"),
    BenchmarkCase("F15", "gearshift_bike", "first_question", ["When are your mobile repair vans open on weekends?"], True, "bike_hours", "Weekend operating hours question"),

    # --------------------------------------------------------------------------
    # Category 3: Follow-Up Questions (12 queries)
    # Expected: Mundane FAQ match
    # --------------------------------------------------------------------------
    BenchmarkCase("U01", "apex_dental", "follow_up", ["Thanks! Do you have parking on site?"], True, "dental_parking_location", "Acknowledgment + parking inquiry"),
    BenchmarkCase("U02", "gearshift_bike", "follow_up", ["Got it, how much does a basic tune-up cost?"], True, "bike_tuneup_cost", "Acknowledgment + tune-up cost"),
    BenchmarkCase("U03", "apex_dental", "follow_up", ["Thank you! What are your business hours on Friday?"], True, "dental_hours", "Thanks + Friday business hours"),
    BenchmarkCase("U04", "apex_dental", "follow_up", ["Awesome, do you take MetLife or Cigna?"], True, "dental_insurance", "Enthusiastic opener + specific insurers"),
    BenchmarkCase("U05", "apex_dental", "follow_up", ["That helps! How much would a dental checkup and cleaning cost without insurance?"], True, "dental_cleaning_cost", "Context acknowledgment + cash price"),
    BenchmarkCase("U06", "bella_roma", "follow_up", ["Great, are you open Sunday mornings?"], True, "bakery_hours", "Follow-up on Sunday schedule"),
    BenchmarkCase("U07", "bella_roma", "follow_up", ["Thanks so much. Do you have any dairy-free or vegan options?"], True, "bakery_vegan", "Thanks + vegan/dairy-free inquiry"),
    BenchmarkCase("U08", "bella_roma", "follow_up", ["Understood! Where exactly is your shop located?"], True, "bakery_location", "Understood + bakery location"),
    BenchmarkCase("U09", "gearshift_bike", "follow_up", ["Cool, do you travel out to Bellevue or Redmond?"], True, "bike_service_area", "Cool + specific Eastside cities"),
    BenchmarkCase("U10", "gearshift_bike", "follow_up", ["Ok got it! How much is an inner tube replacement for a flat tire?"], True, "bike_flat_tire", "Ok got it + tube replacement cost"),
    BenchmarkCase("U11", "gearshift_bike", "follow_up", ["Perfect. Can I schedule a repair online?"], True, "bike_booking_process", "Perfect + online scheduling"),
    BenchmarkCase("U12", "bella_roma", "follow_up", ["Thanks! How much advance notice is required for a celebration cake?"], True, "bakery_custom_cakes", "Thanks + cake notice inquiry"),

    # --------------------------------------------------------------------------
    # Category 4: Multi-Bubble Inputs (12 queries)
    # Expected: Mundane FAQ match
    # --------------------------------------------------------------------------
    BenchmarkCase("M01", "apex_dental", "multi_bubble", ["hello", "quick question", "where are you located?"], True, "dental_parking_location", "3 bubbles: greeting, filler, location"),
    BenchmarkCase("M02", "apex_dental", "multi_bubble", ["hi", "do you guys do teeth whitening?", "and how much does it cost?"], True, "dental_teeth_whitening", "3 bubbles: greeting, whitening, cost"),
    BenchmarkCase("M03", "apex_dental", "multi_bubble", ["hey there", "are you open on weekends?"], True, "dental_hours", "2 bubbles: greeting + weekend hours"),
    BenchmarkCase("M04", "apex_dental", "multi_bubble", ["Hi!", "Do you accept Delta Dental?"], True, "dental_insurance", "2 bubbles: short greeting + insurance"),
    BenchmarkCase("M05", "bella_roma", "multi_bubble", ["good morning", "quick question about your menu", "do you have gluten free pastries?"], True, "bakery_gluten_free", "3 bubbles: greeting + filler + gluten free"),
    BenchmarkCase("M06", "bella_roma", "multi_bubble", ["hey", "what time do you open tomorrow?"], True, "bakery_hours", "2 bubbles: casual greeting + opening time"),
    BenchmarkCase("M07", "bella_roma", "multi_bubble", ["hello!", "can I get catering delivered to my office?", "we are on Market Street"], True, "bakery_delivery", "3 bubbles: greeting + catering + address"),
    BenchmarkCase("M08", "bella_roma", "multi_bubble", ["Hi guys", "do you have any vegan croissants or treats?"], True, "bakery_vegan", "2 bubbles: greeting + vegan treats"),
    BenchmarkCase("M09", "gearshift_bike", "multi_bubble", ["hi", "i need my bike looked at", "what areas do you cover?"], True, "bike_service_area", "3 bubbles: greeting + context + coverage"),
    BenchmarkCase("M10", "gearshift_bike", "multi_bubble", ["hey", "got a flat tire this morning", "can you fix a flat tire today?"], True, "bike_flat_tire", "3 bubbles: greeting + problem + flat tire"),
    BenchmarkCase("M11", "gearshift_bike", "multi_bubble", ["hello", "do you work on ebikes?"], True, "bike_ebike_service", "2 bubbles: greeting + ebike inquiry"),
    BenchmarkCase("M12", "gearshift_bike", "multi_bubble", ["hi there", "how does mobile repair work?", "how do i book?"], True, "bike_booking_process", "3 bubbles: greeting + mechanism + booking"),

    # --------------------------------------------------------------------------
    # Category 5: Complex / Edge Cases / Complaints (15 queries)
    # Expected: SHOULD NOT MATCH Tier 1 (Must escalate to Tier 2 / Human)
    # --------------------------------------------------------------------------
    BenchmarkCase("E01", "apex_dental", "edge_case_negative", ["My tooth broke and it's bleeding, can someone see me right this second?"], False, None, "Acute emergency dental trauma / bleeding"),
    BenchmarkCase("E02", "gearshift_bike", "edge_case_negative", ["I waited 3 hours and nobody showed up, your mechanic is terrible!"], False, None, "Severe no-show customer complaint"),
    BenchmarkCase("E03", "bella_roma", "edge_case_negative", ["I found mold in my sourdough loaf I bought yesterday, I demand a full refund immediately."], False, None, "Food safety violation / refund demand"),
    BenchmarkCase("E04", "apex_dental", "edge_case_negative", ["My root canal from last week is throbbing with severe swelling and unbearable pain."], False, None, "Post-op acute pain & complication"),
    BenchmarkCase("E05", "bella_roma", "edge_case_negative", ["I need to cancel my wedding cake order for next week, our wedding was called off."], False, None, "Contract cancellation / dispute"),
    BenchmarkCase("E06", "gearshift_bike", "edge_case_negative", ["Can you service a 1974 vintage French road bike with non-standard French threading bottom bracket?"], False, None, "Ultra-niche vintage mechanical compatibility"),
    BenchmarkCase("E07", "apex_dental", "edge_case_negative", ["Do you offer full mouth dental implants under general anesthesia for high risk cardiac patients?"], False, None, "Complex surgical / high-risk medical"),
    BenchmarkCase("E08", "bella_roma", "edge_case_negative", ["My daughter has severe airborne celiac disease and peanut anaphylaxis, is your bakery certified allergen-free?"], False, None, "Severe medical allergy / legal liability"),
    BenchmarkCase("E09", "gearshift_bike", "edge_case_negative", ["Your van technician scratched my carbon fiber frame while working on it and I am calling my lawyer."], False, None, "Property damage accusation / legal threat"),
    BenchmarkCase("E10", "apex_dental", "edge_case_negative", ["I need to speak to the clinic owner regarding an unauthorized $800 charge on my credit card."], False, None, "Financial billing dispute / fraud accusation"),
    BenchmarkCase("E11", "gearshift_bike", "edge_case_negative", ["Can you convert my mechanical acoustic road bike into an electric bike with an aftermarket motor kit?"], False, None, "Custom DIY aftermarket conversion request"),
    BenchmarkCase("E12", "bella_roma", "edge_case_negative", ["I sent three messages yesterday and nobody answered, is anyone actually working there?!"], False, None, "Service delivery / communication failure complaint"),
    BenchmarkCase("E13", "bella_roma", "edge_case_negative", ["Do you make sugar-free keto diabetic-safe wedding cakes with erythritol?"], False, None, "Specialty keto medical custom order"),
    BenchmarkCase("E14", "gearshift_bike", "edge_case_negative", ["My brakes completely failed while going downhill after your tune up yesterday, I was almost hit by a bus!"], False, None, "Catastrophic mechanical failure / safety incident"),
    BenchmarkCase("E15", "apex_dental", "edge_case_negative", ["Are you guys hiring dental hygienists right now? I'd like to submit my resume."], False, None, "Employment / job application inquiry"),
]

# ==============================================================================
# 3. Tests against the real matcher
# ==============================================================================

MUNDANE_CASES = [c for c in BENCHMARK_CASES if c.expected_match]
NEGATIVE_CASES = [c for c in BENCHMARK_CASES if not c.expected_match]


def test_dataset_shape():
    assert len(BENCHMARK_CASES) == 66
    assert len(MUNDANE_CASES) == 51
    assert len(NEGATIVE_CASES) == 15


@pytest.mark.parametrize("case", MUNDANE_CASES, ids=lambda c: c.id)
def test_mundane_faq_matches_expected_pattern(case: BenchmarkCase):
    patterns = BUSINESSES[case.business]["patterns"]
    matched, score = match_tier1_patterns(patterns, case.bubbles)
    assert matched is not None, f"{case.id} ({case.description}) did not match any pattern"
    assert matched["id"] == case.expected_pattern_id, (
        f"{case.id} ({case.description}) matched {matched['id']} (score {score:.1f}), "
        f"expected {case.expected_pattern_id}"
    )
    assert score >= 85.0


@pytest.mark.parametrize("case", NEGATIVE_CASES, ids=lambda c: c.id)
def test_edge_cases_are_not_auto_matched(case: BenchmarkCase):
    patterns = BUSINESSES[case.business]["patterns"]
    matched, score = match_tier1_patterns(patterns, case.bubbles)
    assert matched is None, (
        f"{case.id} ({case.description}) inappropriately matched "
        f"{matched['id']} with score {score:.1f}"
    )
    assert score == 0.0
