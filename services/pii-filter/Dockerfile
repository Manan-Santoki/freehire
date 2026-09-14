FROM python:3.13-slim AS model
# Official model repository, pinned so rebuilds use the reviewed weights.
ARG MODEL_REVISION=7ffa9a043d54d1be65afb281eddf0ffbe629385b
RUN python - <<'PY'
import os
from pathlib import Path
from urllib.request import urlretrieve

revision = os.environ["MODEL_REVISION"]
for name in ("config.json", "tokenizer.json", "onnx/model_q4.onnx", "onnx/model_q4.onnx_data"):
    target = Path("/model") / name
    target.parent.mkdir(parents=True, exist_ok=True)
    urlretrieve(f"https://huggingface.co/openai/privacy-filter/resolve/{revision}/{name}", target)
PY

FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt ./
# hadolint ignore=DL3013
RUN pip install --no-cache-dir -r requirements.txt
COPY --from=model /model /model
COPY detector.py server.py ./
ENV PII_FILTER_MODEL_DIR=/model PII_FILTER_ADDR=0.0.0.0:8099
USER 65532:65532
EXPOSE 8099
HEALTHCHECK --interval=30s --timeout=5s --start-period=180s CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8099/health', timeout=4)"]
CMD ["python", "server.py"]
