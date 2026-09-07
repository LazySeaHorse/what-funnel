FROM python:3.11-slim

WORKDIR /app

COPY services/ai-answer-svc/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY packages/python /opt/whatfunnel-python
ENV PYTHONPATH=/opt/whatfunnel-python

COPY services/ai-answer-svc/ .

CMD ["python", "main.py"]
