from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from loguru import logger
from app.config import load_config
from app.api.router import init_routes


def create_app() -> FastAPI:
    config = load_config("configs/config.yaml")

    app = FastAPI(
        title="Bilibili Music API",
        version="2.0.0",
        docs_url="/docs",
        redoc_url="/redoc",
    )

    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    router = init_routes()
    app.include_router(router)

    @app.on_event("startup")
    async def startup():
        logger.info(f"Bilibili Music API starting on {config.server.host}:{config.server.port}")

    @app.on_event("shutdown")
    async def shutdown():
        logger.info("Bilibili Music API shutting down")

    return app


app = create_app()


if __name__ == "__main__":
    import uvicorn
    from app.config import get_config

    cfg = get_config()
    uvicorn.run(
        "app.main:app",
        host=cfg.server.host,
        port=cfg.server.port,
        reload=cfg.server.mode == "debug",
    )
