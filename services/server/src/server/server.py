import os
import socket

import logger
import lottery
import protocol


class Server:
    def __init__(self, server_host: str, server_port: int) -> None:
        self.server_host = server_host
        self.server_port = server_port
        storage_path = os.environ.get("LOTTERY_STORAGE_PATH", "/tmp/lottery-bets.csv")
        self.lottery = lottery.Lottery(storage_path)

    def _handle_client(self, client_socket):
        action = "handle-client"

        agency_id = None
        bets_count = 0

        try:
            logger.info(action, logger.LogResult.in_progress)

            # El primer mensaje de la conexión debe ser HELLO: anuncia la agencia
            # a la que pertenecen todas las apuestas de esta conexión
            hello = protocol.receive_message(client_socket)
            if hello.type != protocol.MessageType.HELLO:
                raise ValueError("first message in a connection must be HELLO")
            agency_id = hello.agency_id

            # Recibir todas las apuestas de la agencia
            while True:
                message = protocol.receive_message(client_socket)

                if message.type == protocol.MessageType.BET:
                    bets_to_store = []
                    for bet_data in message.bets:
                        bet = lottery.Bet(
                            agency_id,
                            bet_data.first_name,
                            bet_data.last_name,
                            bet_data.document,
                            bet_data.birthdate,
                            bet_data.number,
                        )
                        bets_to_store.append(bet)

                    # Persistir el lote de apuestas
                    self.lottery.store_bets(bets_to_store)
                    bets_count += len(bets_to_store)

                    logger.info(
                        "process-bet",
                        logger.LogResult.success,
                        "agency",
                        agency_id,
                        "batch_size",
                        len(bets_to_store),
                    )

                elif message.type == protocol.MessageType.END:
                    break

                else:
                    raise ValueError(
                        f"unexpected message type after HELLO: {message.type}"
                    )

            # Consultar ganadores de esta agencia y responder al cliente
            for stored_bet in self.lottery.load_bets():
                if stored_bet.agency_id == agency_id and self.lottery.has_won(
                    stored_bet
                ):
                    winner_data = protocol.BetData(
                        stored_bet.first_name,
                        stored_bet.last_name,
                        stored_bet.document,
                        stored_bet.birthdate,
                        stored_bet.number,
                    )
                    protocol.send_winner(client_socket, winner_data)

            protocol.send_done(client_socket)
            logger.info(
                action, logger.LogResult.success, "total-bets-processed", bets_count
            )

        except Exception as e:
            logger.error(action, logger.LogResult.fail, "err", str(e))
            raise

    def run(self):
        action = "accept-connection"
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as server_socket:
            server_socket.bind((self.server_host, self.server_port))
            server_socket.listen()

            while True:
                try:
                    logger.info(action, logger.LogResult.in_progress)
                    client_socket, _ = server_socket.accept()
                    logger.info(action, logger.LogResult.success)

                except OSError as e:
                    logger.error(action, logger.LogResult.fail, "err", str(e))
                    continue  # Continúa escuchando a otros clientes sin tirar el servidor

                with client_socket:
                    try:
                        self._handle_client(client_socket)
                    except (OSError, ValueError) as e:
                        logger.error("handle-client", logger.LogResult.fail, "err", str(e))
