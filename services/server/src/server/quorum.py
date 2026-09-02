"""Successive agency-round state owned exclusively by the parent process."""

from dataclasses import dataclass


@dataclass(frozen=True)
class AgencyRound:
    """Exactly one quorum-sized group selected for concurrent processing."""

    number: int
    process_ids: tuple[int, ...]
    agency_ids: tuple[int, ...]


class AgencyRounds:
    """Form exact concurrent rounds from completed, distinct agencies."""

    def __init__(self, minimum: int) -> None:
        if isinstance(minimum, bool) or not isinstance(minimum, int) or minimum <= 0:
            raise ValueError("agency quorum minimum must be a positive integer")

        self._minimum = minimum
        self._waiting: list[tuple[int, int]] = []
        self._process_rounds: dict[int, int] = {}
        self._round_processes: dict[int, set[int]] = {}
        self._last_round_number = 0

    @property
    def waiting_agencies_count(self) -> int:
        """Count distinct agencies currently waiting for a future round."""

        return len({agency_id for _, agency_id in self._waiting})

    def register(self, process_id: int, agency_id: int) -> int:
        """Queue one completed worker and return the distinct waiting count."""

        if process_id in self._process_rounds or any(
            waiting_id == process_id for waiting_id, _ in self._waiting
        ):
            raise ValueError(f"process {process_id} is already registered")

        self._waiting.append((process_id, agency_id))
        return self.waiting_agencies_count

    def start_ready(self) -> list[AgencyRound]:
        """Start every exact quorum round already available in the waiting queue."""

        rounds: list[AgencyRound] = []
        while True:
            selected_indexes: list[int] = []
            selected_agencies: set[int] = set()
            for index, (_, agency_id) in enumerate(self._waiting):
                if agency_id in selected_agencies:
                    continue
                selected_indexes.append(index)
                selected_agencies.add(agency_id)
                if len(selected_indexes) == self._minimum:
                    break

            if len(selected_indexes) < self._minimum:
                return rounds

            selected_index_set = set(selected_indexes)
            selected = [self._waiting[index] for index in selected_indexes]
            self._waiting = [
                registration
                for index, registration in enumerate(self._waiting)
                if index not in selected_index_set
            ]

            self._last_round_number += 1
            round_number = self._last_round_number
            process_ids = tuple(process_id for process_id, _ in selected)
            agency_ids = tuple(agency_id for _, agency_id in selected)
            self._round_processes[round_number] = set(process_ids)
            for process_id in process_ids:
                self._process_rounds[process_id] = round_number
            rounds.append(AgencyRound(round_number, process_ids, agency_ids))

    def remove_process(self, process_id: int) -> int | None:
        """Remove a stopped worker and return its round when that round finishes."""

        self._waiting = [
            registration
            for registration in self._waiting
            if registration[0] != process_id
        ]

        round_number = self._process_rounds.pop(process_id, None)
        if round_number is None:
            return None

        remaining_processes = self._round_processes[round_number]
        remaining_processes.remove(process_id)
        if remaining_processes:
            return None

        del self._round_processes[round_number]
        return round_number
