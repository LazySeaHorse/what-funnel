import uuid
from typing import List, Optional
from pydantic import BaseModel, Field, field_validator
from plain_text import normalize_plain_text


class OKFConceptDraft(BaseModel):
    type: str = Field(description="OKF concept type, e.g. faq, policy, hours, service, pricing")
    title: str = Field(description="Concept title")
    tags: List[str] = Field(default_factory=list, description="Tags associated with the concept")
    body_text: str = Field(description="Plain-text body of the concept; never Markdown or HTML")

    @field_validator("type", "title", "body_text")
    @classmethod
    def plain_fields(cls, value: str) -> str:
        return normalize_plain_text(value)

    @field_validator("tags")
    @classmethod
    def plain_tags(cls, values: List[str]) -> List[str]:
        return [normalize_plain_text(value) for value in values]


class OKFPatternDraft(BaseModel):
    canonical_question: str = Field(description="The standard representative customer question")
    answer_text: str = Field(description="The exact definitive answer in plain text; never Markdown or HTML")
    trigger_phrases: List[str] = Field(
        default_factory=list,
        description="Four to eight realistic lowercase customer query variations"
    )

    @field_validator("canonical_question", "answer_text")
    @classmethod
    def plain_fields(cls, value: str) -> str:
        return normalize_plain_text(value)

    @field_validator("trigger_phrases")
    @classmethod
    def plain_triggers(cls, values: List[str]) -> List[str]:
        return [normalize_plain_text(value) for value in values]


class CompilePasteSchema(BaseModel):
    concepts: List[OKFConceptDraft] = Field(description="List of concepts compiled from raw text")
    patterns: List[OKFPatternDraft] = Field(description="Deterministic customer question and answer patterns")


class CompilePasteRequest(BaseModel):
    raw_text: str


class CompilePasteResponse(BaseModel):
    added_concepts: Optional[List[dict]] = None
    added_patterns: Optional[List[dict]] = None
    suggestion_ids: Optional[List[str]] = None


class CreateIngestionRequest(BaseModel):
    raw_text: str = Field(min_length=1, max_length=500_000)


class PublishIngestionItem(BaseModel):
    id: uuid.UUID
    approved: bool = True
    type: str = Field(min_length=1, max_length=100)
    title: str = Field(min_length=1, max_length=500)
    tags: List[str] = Field(default_factory=list)
    body_text: str = Field(min_length=1, max_length=100_000)


class PublishIngestionPattern(BaseModel):
    id: uuid.UUID
    approved: bool = True
    canonical_question: str = Field(min_length=1, max_length=1000)
    answer_text: str = Field(min_length=1, max_length=100_000)
    trigger_phrases: List[str] = Field(default_factory=list, max_length=50)


class PublishIngestionRequest(BaseModel):
    concepts: List[PublishIngestionItem] = Field(default_factory=list)
    patterns: List[PublishIngestionPattern] = Field(default_factory=list)


class UpdateConceptRequest(BaseModel):
    title: Optional[str] = None
    type: Optional[str] = None
    body_text: Optional[str] = None
    tags: Optional[List[str]] = None


class ApproveSuggestionRequest(BaseModel):
    reviewed_by: str
    edited_payload: Optional[dict] = None


class RejectSuggestionRequest(BaseModel):
    reviewed_by: str


class UpdatePatternRequest(BaseModel):
    canonical_question: Optional[str] = None
    answer_text: Optional[str] = None
    trigger_phrases: Optional[List[str]] = None
