# Sandbox of the WhatsApp Cloud API: records what was sent. A real send
# returns a message id.

def send_template(state, to, template, params):
    state["sent"].append({"to": to, "template": template})
    return {"messages": [{"id": "wamid.%d" % len(state["sent"])}]}

def send_text(state, to, text):
    state["sent"].append({"to": to, "text": text})
    return {"messages": [{"id": "wamid.%d" % len(state["sent"])}]}
