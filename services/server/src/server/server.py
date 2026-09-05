import os
import signal
import socket
import threading
from collections import defaultdict

import logger
import lottery
import protocol


class Server:
    def __init__(
        self, server_host: str, server_port: int, agency_quorum_min: int
    ) -> None:
        self.server_host = server_host
        self.server_port = server_port

        storage_path = os.environ.get("LOTTERY_STORAGE_PATH", "/tmp/lottery-bets.csv")
        self.lottery = lottery.Lottery(storage_path)

        # ------ Estado compartido entre threads ------
        # Protege el acceso concurrente al almacenamiento de apuestas: serializa
        # las escrituras (store_bets) entre sí y frente a la lectura del sorteo
        self._storage_lock = threading.Lock()

        # Barrera de quorum: las agencias que ya notificaron su fin esperan acá
        # hasta que se alcance el minimo configurado para realizar el sorteo
        self._quorum_min = agency_quorum_min
        self._quorum_cond = threading.Condition()
        self._agencies_done = 0

        # Ganadores agrupados por agencia. Se calcula una única vez, cuando el
        # quorum se alcanza, y lo comparten todos los threads
        self._winners_by_agency = None

        # Threads de clientes en curso (para poder unirlos en el shutdown)
        self._threads = []

        # ------ Graceful shutdown ------
        # Se activa al recibir SIGTERM/SIGINT. El socket de escucha se guarda
        # para poder cerrarlo desde el handler y desbloquear el accept()
        self._shutdown = threading.Event()
        self._server_socket = None

    def run(self):
        self._install_signal_handlers()

        action = "accept-connection"
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as server_socket:
            self._server_socket = server_socket
            server_socket.bind((self.server_host, self.server_port))
            server_socket.listen()

            while not self._shutdown.is_set():
                try:
                    logger.info(action, logger.LogResult.in_progress)
                    client_socket, _ = server_socket.accept()
                    logger.info(action, logger.LogResult.success)

                except OSError as e:
                    # Si se está apagando, el accept() se interrumpió por el
                    # cierre del socket: es esperado, sale del loop
                    if self._shutdown.is_set():
                        break
                    logger.error(action, logger.LogResult.fail, "err", str(e))
                    continue  # Continua escuchando a otros clientes sin tirar el servidor

                # Un thread por conexión: permite aceptar y procesar clientes
                # concurrentemente.
                thread = threading.Thread(
                    target=self._handle_client, args=(client_socket,)
                )
                thread.start()
                self._threads.append(thread)

        self._await_threads()

    def _install_signal_handlers(self):
        """Registra el handler de terminacion en el thread principal."""
        signal.signal(signal.SIGTERM, self._on_shutdown_signal)
        signal.signal(signal.SIGINT, self._on_shutdown_signal)

    def _on_shutdown_signal(self, signum, _frame):
        """Inicia el cierre ordenado ante una señal de terminación.

        Marca el flag de shutdown, cierra el socket de escucha (lo que
        interrumpe el accept() bloqueante) y despierta a las agencias que
        estuvieran esperando en la barrera de quorum para que terminen sin
        realizar el sorteo.
        """
        logger.info("shutdown", logger.LogResult.in_progress, "signal", signum)
        self._shutdown.set()

        # Cerrar el socket de escucha desbloquea el accept() del thread principal
        if self._server_socket is not None:
            self._server_socket.close()

        # Desbloquear a las agencias detenidas en la barrera de quorum
        with self._quorum_cond:
            self._quorum_cond.notify_all()

    def _await_threads(self):
        for thread in self._threads:
            thread.join()
        logger.info("shutdown", logger.LogResult.success)

    def _handle_client(self, client_socket):
        action = "handle-client"
        try:
            logger.info(action, logger.LogResult.in_progress)

            with client_socket:
                agency_id = self._receive_hello(client_socket)
                bets_count = self._receive_bets(client_socket, agency_id)

                # Esperar al quorum de agencias antes de sortear
                if not self._await_quorum_and_draw():
                    # Se solicitó el shutdown antes de alcanzar el quorum:
                    # se cierra la conexión sin responder ganadores
                    return

                # Responder a esta agencia únicamente con sus propios ganadores
                self._send_winners(client_socket, agency_id)

            logger.info(
                action, logger.LogResult.success, "total-bets-processed", bets_count
            )

        except (OSError, ValueError) as e:
            logger.error(action, logger.LogResult.fail, "err", str(e))

    def _receive_hello(self, client_socket) -> int:
        hello = protocol.receive_message(client_socket)
        if hello.type != protocol.MessageType.HELLO:
            raise ValueError("first message in a connection must be HELLO")
        return hello.agency_id

    def _receive_bets(self, client_socket, agency_id: int) -> int:
        """Recibe lotes de apuestas hasta el END y los persiste.

        Devuelve la cantidad total de apuestas almacenadas para esta agencia.
        """
        bets_count = 0

        while True:
            message = protocol.receive_message(client_socket)

            if message.type == protocol.MessageType.BET:
                bets_to_store = [
                    lottery.Bet(
                        agency_id,
                        bet_data.first_name,
                        bet_data.last_name,
                        bet_data.document,
                        bet_data.birthdate,
                        bet_data.number,
                    )
                    for bet_data in message.bets
                ]

                # Seccion crítica: varias agencias escriben el mismo archivo
                with self._storage_lock:
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
                raise ValueError(f"unexpected message type after HELLO: {message.type}")

        return bets_count

    def _await_quorum_and_draw(self) -> bool:
        """Barrera de quorum.

        Cada agencia que termina de enviar incrementa el contador. El primer
        thread que alcanza el mínimo realiza el sorteo una única vez y despierta
        al resto. Las agencias que llegan antes esperan; las que lleguen después
        del quorum pasan de largo. Esto soporta el caso de más agencias que el
        quorum sin bloquear a ninguna.

        Devuelve True si se alcanzó el quorum y se puede responder ganadores, o
        False si se solicitó el shutdown mientras la agencia esperaba.
        """
        with self._quorum_cond:
            self._agencies_done += 1

            if self._agencies_done >= self._quorum_min:
                if self._winners_by_agency is None:
                    self._winners_by_agency = self._compute_winners()
                self._quorum_cond.notify_all()
                return True

            # Esperar hasta alcanzar el quorum o hasta que se pida el shutdown
            while self._agencies_done < self._quorum_min and not self._shutdown.is_set():
                self._quorum_cond.wait()

            return not self._shutdown.is_set()

    def _compute_winners(self):
        """Calcula los ganadores agrupados por agencia en una sola lectura.

        Recorre el almacenamiento de forma perezosa (línea a línea, sin cargar
        todo el archivo en memoria) y solo retiene las apuestas ganadoras, que
        son intrinsecamente pocas.

        La lectura completa se realiza bajo `_storage_lock` (el mismo que
        protege las escrituras) para que no se solape con apuestas que pudiera
        seguir cargando una agencia ajena al quorum. Al ser una única pasada, el
        costo de mantener el lock es mínimo. Las agencias que forman el quorum ya
        terminaron de escribir antes de contar para él, por lo que sus ganadores
        salen completos y de forma determinista.
        """
        winners_by_agency = defaultdict(list)
        with self._storage_lock:
            for stored_bet in self.lottery.load_bets():
                if self.lottery.has_won(stored_bet):
                    winners_by_agency[stored_bet.agency_id].append(
                        protocol.BetData(
                            stored_bet.first_name,
                            stored_bet.last_name,
                            stored_bet.document,
                            stored_bet.birthdate,
                            stored_bet.number,
                        )
                    )
        return winners_by_agency

    def _send_winners(self, client_socket, agency_id: int) -> None:
        for winner in self._winners_by_agency.get(agency_id, []):
            protocol.send_winner(client_socket, winner)
        protocol.send_done(client_socket)
