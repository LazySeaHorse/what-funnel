"""
Unit tests for AI Cascade Tier 1 Safety & Escalation Guard (matcher.py).
Tests:
- ESCALATION_PATTERNS definitions and regex compilation
- is_escalation() detection across 5 core risk categories:
    1. Acute medical emergencies, severe distress, and physical trauma
    2. Severe service complaints, failures, no-shows, and property damage
    3. Financial / contractual disputes, unauthorized charges, and refund/cancellation demands
    4. Legal threats (lawyers, lawsuits, legal action)
    5. Out-of-scope domain requests and employment inquiries (hiring, resume, anesthesia, vintage threading)
- Calibration ensuring mundane questions containing innocent words are cleanly distinguished:
    - "Do you offer emergency dental appointments?" -> NOT escalated
    - "What is your cancellation policy?" -> NOT escalated
    - "Do you take after-hours jobs?" -> NOT escalated
    - "What is your refund policy?" -> NOT escalated
- Integration with match_tier1_patterns: immediate rejection (None, 0.0) on escalated queries
"""

import pytest
from matcher import (
    ESCALATION_PATTERNS,
    is_escalation,
    match_tier1_patterns,
)

# Sample test patterns for integration testing
SAMPLE_PATTERNS = [
    {
        "id": "dental_teeth_whitening",
        "trigger_phrases": [
            "teeth whitening",
            "how much is teeth whitening",
            "teeth whitening cost",
            "do you offer teeth whitening",
        ],
        "answer_text": "We offer in-office teeth whitening for $399.",
    },
    {
        "id": "dental_emergency_policy",
        "canonical_question": "Do you offer emergency dental appointments?",
        "trigger_phrases": [
            "emergency dental appointments",
            "emergency appointments",
            "do you offer emergency dental appointments",
            "emergency dentist",
            "emergency appointment",
        ],
        "answer_text": "We reserve same-day emergency slots for dental emergencies.",
    },
    {
        "id": "dental_hours",
        "trigger_phrases": [
            "what are your hours",
            "clinic hours",
            "opening hours",
            "business hours",
        ],
        "answer_text": "We are open Monday to Friday 8am to 6pm.",
    },
    {
        "id": "dental_parking_location",
        "trigger_phrases": [
            "where are you located",
            "clinic address",
            "parking options",
            "where is your office",
        ],
        "answer_text": "We are located at 450 Sutter St with underground parking.",
    },
]


# ==============================================================================
# 1. Acute Medical Emergencies, Severe Distress, & Trauma
# ==============================================================================

class TestAcuteMedicalEmergencies:
    """Tests for acute distress, bleeding, broken teeth, severe pain, anaphylaxis, poison."""

    @pytest.mark.parametrize(
        "emergency_msg",
        [
            "My tooth broke and it's bleeding, can someone see me right this second?",
            "My gums have been bleeding non-stop since this morning.",
            "My root canal from last week is throbbing with severe swelling and unbearable pain.",
            "I am in acute pain and my jaw is swelling rapidly.",
            "I'm experiencing excruciating pain in my lower molar.",
            "My front tooth broke off when I bit into something hard.",
            "I broke my tooth during basketball practice.",
            "I sustained a severe facial injury and need immediate care.",
            "Help, my daughter has peanut anaphylaxis!",
            "My child accidentally swallowed household cleaner, suspect poisoning.",
            "Is your bakery certified allergen-free? My son has airborne celiac disease.",
            "This is an emergency! Someone please help me.",
            "I am having a medical emergency right now.",
            "Emergency! I need a doctor immediately.",
        ],
    )
    def test_detects_acute_emergencies(self, emergency_msg: str):
        assert is_escalation(emergency_msg) is True, f"Failed to escalate acute emergency: {emergency_msg}"

    @pytest.mark.parametrize(
        "innocent_msg",
        [
            "Do you offer emergency dental appointments?",
            "Do you have emergency dental services?",
            "What is your emergency policy?",
            "Do you have an emergency dentist on staff?",
            "What are your emergency hours?",
            "Can I schedule an emergency appointment for dental care?",
        ],
    )
    def test_cleanly_distinguishes_innocent_emergency_inquiries(self, innocent_msg: str):
        assert is_escalation(innocent_msg) is False, f"Falsely escalated innocent emergency question: {innocent_msg}"


# ==============================================================================
# 2. Severe Service Failures, No-Shows, & Property Damage
# ==============================================================================

class TestSevereServiceFailures:
    """Tests for no-shows, multi-hour wait times, property damage, and catastrophic failure."""

    @pytest.mark.parametrize(
        "complaint_msg",
        [
            "I waited 3 hours and nobody showed up, your mechanic is terrible!",
            "Waited 2 hours outside and your technician never showed up.",
            "This was a complete no-show for our scheduled window.",
            "Nobody showed up for the 10am booking!",
            "I sent three messages yesterday and nobody answered, is anyone actually working there?!",
            "Your customer support is terrible and completely unresponsive.",
            "This service was horrible and a total waste of money.",
            "Your technician's behavior was completely unacceptable.",
            "This is a catastrophic failure of your mobile dispatch.",
            "My brakes completely failed while going downhill after your tune up yesterday, I was almost hit by a bus!",
            "Your van technician scratched my carbon fiber frame while working on it.",
            "I want to file a formal complaint against the mechanic who worked on my bike.",
        ],
    )
    def test_detects_severe_complaints_and_failures(self, complaint_msg: str):
        assert is_escalation(complaint_msg) is True, f"Failed to escalate severe complaint: {complaint_msg}"

    @pytest.mark.parametrize(
        "innocent_msg",
        [
            "Do you guys do brake tune ups?",
            "How much does it cost to adjust my brakes?",
            "Can you tune up my bicycle this afternoon?",
            "What time do you usually arrive for appointments?",
        ],
    )
    def test_innocent_service_questions_not_escalated(self, innocent_msg: str):
        assert is_escalation(innocent_msg) is False, f"Falsely escalated innocent service question: {innocent_msg}"


# ==============================================================================
# 3. Financial & Contractual Disputes, Refund & Cancellation Demands
# ==============================================================================

class TestFinancialDisputesAndRefunds:
    """Tests for refunds, fraudulent/unauthorized charges, and active cancellations."""

    @pytest.mark.parametrize(
        "dispute_msg",
        [
            "I found mold in my sourdough loaf I bought yesterday, I demand a full refund immediately.",
            "I want a refund for the service yesterday, it didn't work.",
            "Please issue a refund to my credit card immediately.",
            "Give me a refund right now.",
            "I need to speak to the clinic owner regarding an unauthorized $800 charge on my credit card.",
            "There is a fraudulent charge of $250 from your store on my statement.",
            "I am disputing this charge with my bank.",
            "You overcharged me by $100 on my invoice.",
            "I need to cancel my wedding cake order for next week, our wedding was called off.",
            "Please cancel my appointment for tomorrow at 2pm.",
            "I want to cancel my order immediately.",
            "I would like to cancel our contract.",
            "Cancellation request for order #9821.",
        ],
    )
    def test_detects_financial_disputes_and_cancellations(self, dispute_msg: str):
        assert is_escalation(dispute_msg) is True, f"Failed to escalate financial dispute: {dispute_msg}"

    @pytest.mark.parametrize(
        "innocent_msg",
        [
            "What is your cancellation policy?",
            "Can you tell me your cancellation policy?",
            "What are your cancellation terms?",
            "What is your cancellation fee if I reschedule?",
            "What is your refund policy?",
            "Do you have a refund policy for custom orders?",
        ],
    )
    def test_cleanly_distinguishes_innocent_policy_questions(self, innocent_msg: str):
        assert is_escalation(innocent_msg) is False, f"Falsely escalated innocent policy inquiry: {innocent_msg}"


# ==============================================================================
# 4. Legal Threats
# ==============================================================================

class TestLegalThreats:
    """Tests for attorney, lawyer, sue, court, and legal action threats."""

    @pytest.mark.parametrize(
        "legal_msg",
        [
            "Your van technician scratched my carbon fiber frame while working on it and I am calling my lawyer.",
            "My lawyer will be reaching out to your legal department tomorrow.",
            "I am going to sue your clinic for dental malpractice.",
            "We will sue your company if this is not resolved today.",
            "I am taking legal action against your business.",
            "I have retained legal counsel regarding this breach of contract.",
            "My attorney will contact you shortly.",
            "I will take you to court over this damage.",
            "See you in court!",
        ],
    )
    def test_detects_legal_threats(self, legal_msg: str):
        assert is_escalation(legal_msg) is True, f"Failed to escalate legal threat: {legal_msg}"

    @pytest.mark.parametrize(
        "innocent_msg",
        [
            "Are you located near the state supreme court building?",
            "Do you provide dental services to corporate legal departments?",
        ],
    )
    def test_innocent_legal_words_not_escalated(self, innocent_msg: str):
        assert is_escalation(innocent_msg) is False, f"Falsely escalated innocent question: {innocent_msg}"


# ==============================================================================
# 5. Out-of-Scope Domain Requests & Employment Inquiries
# ==============================================================================

class TestOutOfScopeAndEmployment:
    """Tests for hiring, resumes, job applications, ultra-niche mechanics, and extreme diets."""

    @pytest.mark.parametrize(
        "out_of_scope_msg",
        [
            "Are you guys hiring dental hygienists right now? I'd like to submit my resume.",
            "Are you hiring bike mechanics for the summer season?",
            "I'd like to submit my resume for your open baker position.",
            "Where can I send my resume?",
            "Do you have any job openings right now?",
            "I want to apply for a job at your bakery.",
            "I'm looking for a job as an apprentice.",
            "Do you offer full mouth dental implants under general anesthesia for high risk cardiac patients?",
            "Can you service a 1974 vintage French road bike with non-standard French threading bottom bracket?",
            "Can you convert my mechanical acoustic road bike into an electric bike with an aftermarket motor kit?",
            "Do you make sugar-free keto diabetic-safe wedding cakes with erythritol?",
            "I found mold in my bread.",
        ],
    )
    def test_detects_out_of_scope_and_employment(self, out_of_scope_msg: str):
        assert is_escalation(out_of_scope_msg) is True, f"Failed to escalate out-of-scope query: {out_of_scope_msg}"

    @pytest.mark.parametrize(
        "innocent_job_msg",
        [
            "Do you take after-hours jobs?",
            "How much does a brake job cost?",
            "Can you handle a custom paint job on a road bike frame?",
            "Is this repair job too big for mobile van service?",
            "How long does a routine cleaning job take?",
        ],
    )
    def test_cleanly_distinguishes_innocent_job_queries(self, innocent_job_msg: str):
        assert is_escalation(innocent_job_msg) is False, f"Falsely escalated innocent 'job' question: {innocent_job_msg}"


# ==============================================================================
# 6. Input Formats & Robustness
# ==============================================================================

class TestIsEscalationInputHandling:
    """Tests input handling for string, multi-bubble lists, case insensitivity, and empty inputs."""

    def test_handles_single_string(self):
        assert is_escalation("I demand a full refund") is True
        assert is_escalation("What are your clinic hours?") is False

    def test_handles_multi_bubble_list(self):
        bubbles = ["hello", "quick question", "I am going to sue you"]
        assert is_escalation(bubbles) is True

    def test_multi_bubble_innocent(self):
        bubbles = ["hello there", "quick question", "where are you located?"]
        assert is_escalation(bubbles) is False

    def test_handles_empty_inputs(self):
        assert is_escalation("") is False
        assert is_escalation([]) is False
        assert is_escalation(["", "   "]) is False
        assert is_escalation(None) is False

    def test_case_insensitivity(self):
        assert is_escalation("I DEMAND A REFUND IMMEDIATELY") is True
        assert is_escalation("My Tooth Broke And It Is Bleeding") is True
        assert is_escalation("CALLING MY LAWYER") is True
        assert is_escalation("DO YOU TAKE AFTER-HOURS JOBS?") is False


# ==============================================================================
# 7. Integration: match_tier1_patterns with Escalation Guard
# ==============================================================================

class TestMatchTier1PatternsWithEscalationGuard:
    """Verifies that match_tier1_patterns immediately returns (None, 0.0) when escalated."""

    def test_immediate_rejection_on_acute_emergency_with_faq_keywords(self):
        # Inbound contains trigger words "teeth whitening" but also acute trauma
        bubbles = ["My tooth broke and it's bleeding, how much is teeth whitening?"]
        pat, score = match_tier1_patterns(SAMPLE_PATTERNS, bubbles)
        assert pat is None
        assert score == 0.0

    def test_immediate_rejection_on_service_complaint_with_faq_keywords(self):
        # Inbound contains "clinic hours" but also severe service failure
        bubbles = ["I waited 3 hours and nobody showed up, what are your clinic hours anyway?"]
        pat, score = match_tier1_patterns(SAMPLE_PATTERNS, bubbles)
        assert pat is None
        assert score == 0.0

    def test_immediate_rejection_on_refund_demand_with_faq_keywords(self):
        # Inbound contains "teeth whitening" but also refund demand
        bubbles = ["I want a full refund for my teeth whitening treatment yesterday!"]
        pat, score = match_tier1_patterns(SAMPLE_PATTERNS, bubbles)
        assert pat is None
        assert score == 0.0

    def test_immediate_rejection_on_legal_threat_with_faq_keywords(self):
        # Inbound contains "where are you located" but also legal threat
        bubbles = ["Your mechanic scratched my bike, calling my lawyer. Where are you located?"]
        pat, score = match_tier1_patterns(SAMPLE_PATTERNS, bubbles)
        assert pat is None
        assert score == 0.0

    def test_immediate_rejection_on_hiring_with_faq_keywords(self):
        # Inbound contains "clinic hours" but also hiring inquiry
        bubbles = ["Are you guys hiring? What are your clinic hours?"]
        pat, score = match_tier1_patterns(SAMPLE_PATTERNS, bubbles)
        assert pat is None
        assert score == 0.0

    def test_innocent_emergency_appointment_inquiry_matches_pattern(self):
        # "Do you offer emergency dental appointments?" should NOT be rejected,
        # it should cleanly match the dental_emergency_policy pattern
        bubbles = ["Do you offer emergency dental appointments?"]
        pat, score = match_tier1_patterns(SAMPLE_PATTERNS, bubbles)
        assert pat is not None
        assert pat["id"] == "dental_emergency_policy"
        assert score >= 85.0

    def test_innocent_faq_queries_continue_to_match(self):
        # Standard FAQ query matches normally
        bubbles = ["What are your clinic hours?"]
        pat, score = match_tier1_patterns(SAMPLE_PATTERNS, bubbles)
        assert pat is not None
        assert pat["id"] == "dental_hours"
        assert score >= 85.0
