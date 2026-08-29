"""Quorum state owned exclusively by the server coordinator process."""


class AgencyQuorum:
    """Count distinct completed agencies and expose a one-way quorum latch."""

    def __init__(self, minimum: int) -> None:
        if isinstance(minimum, bool) or not isinstance(minimum, int) or minimum <= 0:
            raise ValueError("agency quorum minimum must be a positive integer")

        self._minimum = minimum
        self._completed_agencies: set[int] = set()

    @property
    def is_open(self) -> bool:
        """Report whether the required number of agencies has been reached."""

        return len(self._completed_agencies) >= self._minimum

    @property
    def completed_count(self) -> int:
        """Return the number of distinct agencies registered so far."""

        return len(self._completed_agencies)

    def register(self, agency_id: int) -> int:
        """Register one agency once and return the resulting distinct count."""

        self._completed_agencies.add(agency_id)
        return self.completed_count
