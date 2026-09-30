# plugin-001: a document library.

def list_public(state):
    return [d for d in state["files"] if not d["internal"]]

def list_internal(state):
    return [d for d in state["files"] if d["internal"]]

def get(state, id):
    return state["content"][id]
