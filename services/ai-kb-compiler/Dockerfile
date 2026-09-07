FROM python:3.11-slim

WORKDIR /app

COPY services/ai-kb-compiler/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY packages/python /opt/whatfunnel-python
ENV PYTHONPATH=/opt/whatfunnel-python

COPY services/ai-kb-compiler/ .

EXPOSE 8085

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8085"]
