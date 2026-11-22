FROM python:3.13-slim-bookworm
WORKDIR /bot
COPY mechfight.py arena.py bot.py ./
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
ENTRYPOINT ["python3", "-u", "/bot/bot.py"]
