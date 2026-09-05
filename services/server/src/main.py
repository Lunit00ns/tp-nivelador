import sys

import config
import logger
import server


def main():
    logger.init()

    try:
        cfg = config.load_config()
    except config.ConfigError as e:
        logger.error("load-config", logger.LogResult.fail, "err", str(e))
        return 1

    s = server.Server(cfg.host, cfg.port, cfg.agency_quorum_min)
    try:
        s.run()
    except Exception as e:
        logger.error("server-run", logger.LogResult.fail, "err", e)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
