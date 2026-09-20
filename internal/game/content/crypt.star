def on_enter(event):
    if event.room.name == "Crypt Lantern Stair":
        tell("The Bellkeeper's trial rewards memory. Read the side chambers before answering in the reliquary.")

def on_command(event):
    if event.command != "answer" or event.room.name != "Bellkeeper Reliquary":
        return False
    if get_state("completed") == "yes":
        tell("The Bellkeeper has already entrusted you with an offering.")
    elif event.text.strip().lower() in ["bell", "a bell", "the bell"]:
        set_state("completed", "yes")
        award_gold(30)
        tell("A clear note rings out. The offering bowl opens: you receive 30 gold and the Bellkeeper's thanks.")
    else:
        tell("The bowl remains sealed. Seek the verses in the choir and the scribe's resting place.")
    return True
