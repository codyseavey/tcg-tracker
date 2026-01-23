# Identifier service Dockerfile
# Supports both CPU-only and GPU modes via build args

ARG BASE_IMAGE=python:3.11-slim

FROM ${BASE_IMAGE}

WORKDIR /app

# Install system dependencies for OpenCV (used by EasyOCR)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements first for better layer caching
COPY requirements.txt .

# Install Python dependencies
# Use CUDA 11.8 PyTorch for broader GPU support (Pascal/GTX 1080 Ti and newer)
RUN pip install --no-cache-dir -r requirements.txt \
    && pip install --no-cache-dir torch torchvision --index-url https://download.pytorch.org/whl/cu118

# Copy application code
COPY . .

# Install the package in editable mode so 'identifier' module is importable
RUN pip install --no-cache-dir -e .

# Environment variables
ENV HOST=0.0.0.0
ENV PORT=8099
ENV PYTHONUNBUFFERED=1

# Expose port
EXPOSE 8099

# Default command (can be overridden in docker-compose)
CMD ["uvicorn", "identifier.app:app", "--host", "0.0.0.0", "--port", "8099"]
