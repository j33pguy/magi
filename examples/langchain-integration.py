#!/usr/bin/env python3
"""Conceptual LangChain integration using MAGI as a memory backend.

This is illustrative and may need tweaks depending on your LangChain version.
"""

from __future__ import annotations

import os
from typing import Any, Dict, List

import requests
from langchain_core.memory import BaseMemory
from langchain_core.messages import AIMessage, BaseMessage, HumanMessage

MAGI_HTTP_URL = os.getenv("MAGI_HTTP_URL", "http://localhost:8302")
MAGI_API_TOKEN = os.getenv("MAGI_API_TOKEN", "")


def _headers() -> Dict[str, str]:
    headers = {"Content-Type": "application/json"}
    if MAGI_API_TOKEN:
        headers["Authorization"] = f"Bearer {MAGI_API_TOKEN}"
    return headers


class MagiMemory(BaseMemory):
    """Minimal memory wrapper that stores conversation turns in MAGI."""

    def __init__(self, project: str = "langchain") -> None:
        super().__init__()
        self.project = project

    @property
    def memory_variables(self) -> List[str]:
        return ["history"]

    def load_memory_variables(self, inputs: Dict[str, Any]) -> Dict[str, Any]:
        query = inputs.get("input", "")
        resp = requests.post(
            f"{MAGI_HTTP_URL}/recall",
            headers=_headers(),
            json={"query": query, "project": self.project, "top_k": 5},
            timeout=10,
        )
        resp.raise_for_status()
        data = resp.json()
        return {"history": data}

    def save_context(self, inputs: Dict[str, Any], outputs: Dict[str, Any]) -> None:
        user_text = inputs.get("input", "")
        assistant_text = outputs.get("output", "")
        payload = {
            "content": f"User: {user_text}\nAssistant: {assistant_text}",
            "type": "conversation",
            "project": self.project,
            "speaker": "langchain",
            "tags": ["conversation"],
        }
        resp = requests.post(
            f"{MAGI_HTTP_URL}/remember",
            headers=_headers(),
            json=payload,
            timeout=10,
        )
        resp.raise_for_status()

    def clear(self) -> None:
        # MAGI has no clear-by-project endpoint yet; this is a no-op.
        pass


# Example usage with a chain or runnable:
if __name__ == "__main__":
    memory = MagiMemory(project="demo")

    # Simulate a turn for demonstration.
    memory.save_context({"input": "What did we decide about the API?"}, {"output": "Use v3 endpoints."})

    recalled = memory.load_memory_variables({"input": "API decision"})
    print("Recalled memory:")
    print(recalled)

    # If you want to translate into LangChain messages:
    messages: List[BaseMessage] = [
        HumanMessage(content="API decision?"),
        AIMessage(content="Use v3 endpoints."),
    ]
    print("Messages:", messages)
