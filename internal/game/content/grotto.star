def on_enter(event):
    if event.room.name == "Tideglass Steps":
        tell("Stay on the dry ledges. The shell archive preserves the lesson needed to restore the lens.")

def on_command(event):
    if event.command != "align" or event.room.name != "Tideglass Lens Chamber":
        return False
    if get_state("completed") == "yes":
        tell("Your lens shines steadily. You have already claimed the keeper's gratitude.")
        return True
    order = ["moon", "tide", "beacon"]
    choice = event.text.strip().lower()
    if choice not in order:
        tell("Use align moon, align tide, or align beacon.")
        return True
    step = int(get_state("step", "0"))
    if choice != order[step]:
        set_state("step", "0")
        tell("The light scatters. Begin again in the order recorded by the shells.")
        return True
    step += 1
    if step == len(order):
        set_state("completed", "yes")
        set_state("step", "0")
        award_gold(50)
        tell("A ribbon of light reaches the sea. A keeper's cache opens, granting you 50 gold.")
    else:
        set_state("step", str(step))
        tell("The symbol brightens. Align the next symbol.")
    return True
