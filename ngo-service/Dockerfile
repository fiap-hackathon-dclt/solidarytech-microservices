FROM python:3.13.18-alpine3.24@sha256:9ef1a599f863231c3d91e941389bfdc4a2d6070557243d54df1a4e57015b960e AS build
WORKDIR /build
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --root-user-action=ignore --prefix=/install -r requirements.txt

FROM python:3.13.18-alpine3.24@sha256:9ef1a599f863231c3d91e941389bfdc4a2d6070557243d54df1a4e57015b960e
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1
RUN pip uninstall -y --root-user-action=ignore pip \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
WORKDIR /app
COPY --from=build /install /usr/local
COPY app.py .
USER 10001:10001
EXPOSE 8081
CMD ["gunicorn", "--bind", "0.0.0.0:8081", "--no-control-socket", "app:app"]
