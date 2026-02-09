FROM python:3.11-slim

WORKDIR /app

RUN apt-get update && apt-get install -y \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir nicegui>=2.0.0 httpx>=0.27.0

COPY nicegui_app.py .

EXPOSE 8502

CMD ["python", "nicegui_app.py"]
