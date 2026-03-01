FROM python:3.12-slim

WORKDIR /app
COPY ./strategy/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY ./strategy .
