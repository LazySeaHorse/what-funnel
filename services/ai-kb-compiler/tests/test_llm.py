import pytest
from unittest.mock import patch, AsyncMock, MagicMock
from pydantic import BaseModel
from fastapi import HTTPException
from llm import embed, complete

# Test Schema
class MockConcept(BaseModel):
    title: str
    body: str
    tags: list[str]

@pytest.mark.asyncio
async def test_embed_success():
    mock_response = AsyncMock()
    mock_response.status_code = 200
    mock_response.raise_for_status = MagicMock()
    mock_response.json = MagicMock(return_value={
        "data": [
            {
                "embedding": [0.1] * 1536
            }
        ]
    })

    with patch("httpx.AsyncClient.post", return_value=mock_response) as mock_post:
        result = await embed("test-key", "https://api.openai.com/v1", "text-embedding-3-small", "hello")
        assert len(result) == 1536
        assert result[0] == 0.1
        mock_post.assert_called_once()

@pytest.mark.asyncio
async def test_embed_invalid_dimension():
    mock_response = AsyncMock()
    mock_response.status_code = 200
    mock_response.raise_for_status = MagicMock()
    mock_response.json = MagicMock(return_value={
        "data": [
            {
                "embedding": [0.1] * 512 # Invalid dimension
            }
        ]
    })

    with patch("httpx.AsyncClient.post", return_value=mock_response):
        with pytest.raises(HTTPException) as exc_info:
            await embed("test-key", "https://api.openai.com/v1", "text-embedding-3-small", "hello")
        assert exc_info.value.status_code == 400
        assert "Unsupported embedding dimension" in exc_info.value.detail

@pytest.mark.asyncio
async def test_complete_success():
    mock_response = AsyncMock()
    mock_response.status_code = 200
    mock_response.raise_for_status = MagicMock()
    mock_response.json = MagicMock(return_value={
        "choices": [
            {
                "message": {
                    "content": '{"title": "Test FAQ", "body": "FAQ content here", "tags": ["test"]}'
                }
            }
        ]
    })

    with patch("httpx.AsyncClient.post", return_value=mock_response) as mock_post:
        result = await complete(
            "test-key", 
            "https://api.openai.com/v1", 
            "gpt-4o-mini", 
            "Draft an FAQ", 
            MockConcept
        )
        assert result["title"] == "Test FAQ"
        assert result["body"] == "FAQ content here"
        assert result["tags"] == ["test"]
        mock_post.assert_called_once()

@pytest.mark.asyncio
async def test_complete_validation_failure():
    mock_response = AsyncMock()
    mock_response.status_code = 200
    mock_response.raise_for_status = MagicMock()
    mock_response.json = MagicMock(return_value={
        "choices": [
            {
                "message": {
                    "content": '{"title": "Test FAQ", "body": "FAQ content here"}' # missing tags
                }
            }
        ]
    })

    with patch("httpx.AsyncClient.post", return_value=mock_response):
        with pytest.raises(HTTPException) as exc_info:
            await complete(
                "test-key", 
                "https://api.openai.com/v1", 
                "gpt-4o-mini", 
                "Draft an FAQ", 
                MockConcept
            )
        assert exc_info.value.status_code == 502
        assert "response failed schema validation" in exc_info.value.detail
