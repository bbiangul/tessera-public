# web/search in tests (websearch-001, W7): fixed, harmless results, so a
# free turn that searches never reaches a real provider.
def search(state, query, **kw):
    return state["results"]
