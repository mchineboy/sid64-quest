def build():
    # The first room is the world root. link() always creates a return exit.
    room("square", "Town Square", "A fountain murmurs over cobblestones. The gate is north, the market east, the inn south, and the old town west. Type trails for an explorer's guide.", "safe", "guide")
    room("gate", "North Gate", "Weathered gates frame the docks to the north. East, an old road follows the river toward farms and distant hills.", "safe")
    room("market", "Market Lane", "Canvas awnings snap overhead. Beyond the stalls to the east lies a garden of medicinal herbs.", "shop")
    room("inn", "The Prancing Pony Inn", "A low fire and a room full of rumors welcome travelers. Rest here before following the southern road.", "inn")
    room("docks", "Moonlit Docks", "Black water taps against the pilings. The harbor ledger lies somewhere nearby. A seawall runs east toward a lonely lighthouse.")
    link("square", "north", "gate")
    link("square", "east", "market")
    link("square", "south", "inn")
    link("gate", "north", "docks")

    # Overworld: three trails with a return loop through the countryside.
    for key, name, description, kind in [
        ("old_town", "Old Town", "Narrow houses lean over a lane west of the square. Beyond them a cemetery climbs toward the hills.", "safe"),
        ("cemetery", "Lantern Cemetery", "Blue lanterns mark old graves. A stair descends into the Bellkeeper Crypt; a carved sign promises a trial of memory, not strength.", "normal"),
        ("river_road", "River Road", "Willows trail their fingers in the river. North lies a stone bridge; east, a mill turns beside the water.", "normal"),
        ("bridge", "Willow Bridge", "A broad bridge crosses the silver river. Farm tracks continue north, while the river road leads home to the south.", "normal"),
        ("orchard", "Windfall Orchard", "Apples scent the air. The trees thin northward into whispering woodland; a distant ridge rises above the canopy.", "normal"),
        ("mill", "Mosswheel Mill", "A waterwheel creaks through green water. Flour-dusted footprints lead east toward a woodland lodge.", "normal"),
        ("lodge", "Forester's Lodge", "A kettle steams above a hearth. Travelers may rest here. A trail north reaches a spring, and west lies the mill.", "inn"),
        ("forest", "Whispering Wood", "Leaves hiss like distant voices. A ridge rises north of the trail; to the east, a spring glints between the roots.", "normal"),
        ("spring", "Starlit Spring", "Clear water reflects the sky even beneath the canopy. An eastern trail climbs to a ruined watchtower.", "normal"),
        ("tower", "Broken Watchtower", "From the broken battlements you can trace the river to town and the ridge to an abandoned silver mine.", "normal"),
        ("ridge", "Heather Ridge", "Purple heather bows before a cold wind. East, a mine entrance yawns beneath a rusted bell.", "normal"),
        ("mine_entrance", "Silvervein Mine Entrance", "An abandoned lift descends into Silvervein Mine. A notice reads: Restore the pumps, recover the survey, leave the workings intact.", "normal"),
        ("garden", "Apothecary Garden", "Mint and lavender grow in careful rows. The market lies west; bees hum over the flowerbeds.", "safe"),
        ("common", "South Common", "Kites wheel over open grass. The inn is north, and a coastal path runs south.", "safe"),
        ("coast", "Saltwind Path", "Sea thrift grows between pale stones. Eastward, the path meets the lighthouse keeper's cottage.", "normal"),
        ("seawall", "Harbor Seawall", "Spray leaps against the wall. The docks lie west and the lighthouse rises east above a cave-pierced headland.", "normal"),
        ("lighthouse", "Lighthouse Headland", "A beacon sweeps the sea. Steps descend into the Tideglass Grotto. South stands the keeper's welcoming cottage.", "normal"),
        ("cottage", "Keeper's Cottage", "The keeper has left a fire and spare blankets for travelers. Rest here before exploring the grotto beneath the lighthouse.", "inn"),
    ]:
        room(key, name, description, kind)
    link("square", "west", "old_town")
    link("old_town", "west", "cemetery")
    link("gate", "east", "river_road")
    link("river_road", "north", "bridge")
    link("bridge", "north", "orchard")
    link("river_road", "east", "mill")
    link("mill", "east", "lodge")
    link("lodge", "north", "spring")
    link("orchard", "north", "forest")
    link("forest", "east", "spring")
    link("spring", "east", "tower")
    link("forest", "north", "ridge")
    link("ridge", "east", "mine_entrance")
    link("market", "east", "garden")
    link("inn", "south", "common")
    link("common", "south", "coast")
    link("coast", "east", "cottage")
    link("docks", "east", "seawall")
    link("seawall", "east", "lighthouse")
    link("lighthouse", "south", "cottage")

    # Each dungeon has a safe return stair, side chambers with clues, and a finale.
    for key, name, description in [
        ("crypt_stair", "Crypt Lantern Stair", "Blue lanterns light dry steps. Up returns to the cemetery; north leads into the Bellkeeper Crypt. No door closes behind you."),
        ("crypt_hall", "Processional Hall", "Stone mourners face a northern reliquary. An eastern arch leads to the choir, and a western arch to a scribe's resting place."),
        ("crypt_choir", "Silent Choir", "A mural depicts a bronze bell sending ripples through the air. Beneath it: I have a tongue, but never speak."),
        ("crypt_scribe", "Scribe's Rest", "A slate reads: I call the town from sleep, yet never wake myself. The answer waits in the reliquary to the north of the hall."),
        ("crypt_reliquary", "Bellkeeper Reliquary", "A sealed offering bowl bears a riddle: With a tongue of bronze I call the town, yet have no breath. Answer with: answer <word>."),
        ("crypt_ossuary", "Garden of Bones", "Tiny white flowers grow among ancient stones. A plaque asks visitors to leave the sleepers undisturbed. The reliquary lies west."),
    ]:
        room(key, name, description, "dungeon", "crypt")
    link("cemetery", "down", "crypt_stair")
    link("crypt_stair", "north", "crypt_hall")
    link("crypt_hall", "east", "crypt_choir")
    link("crypt_hall", "west", "crypt_scribe")
    link("crypt_hall", "north", "crypt_reliquary")
    link("crypt_reliquary", "east", "crypt_ossuary")
    link("crypt_choir", "north", "crypt_ossuary")

    for key, name, description in [
        ("mine_lift", "Silvervein Lift Landing", "The lift rests on solid stone. Up returns to daylight. North, rail tracks lead to a junction."),
        ("mine_junction", "Ore Cart Junction", "Rails branch east to a survey office, west to a flooded gallery, and north to the pumping chamber."),
        ("mine_office", "Survey Office", "A pinned diagram reads: First open the INTAKE. Then engage the WHEEL. Only then release the SLUICE. The pumps lie north of the junction."),
        ("mine_gallery", "Flooded Gallery", "Still water covers the lower workings. A warning reads: A wrong pump setting resets the sequence. No need to enter the water."),
        ("mine_pumps", "Silvervein Pump Chamber", "Three brass controls are labeled intake, wheel, and sluice. Operate them with crank <label>. A locked survey case is connected to the pump."),
        ("mine_seam", "Crystal Seam", "Unmined crystals scatter lamplight like stars. An old miner's verse praises the beauty of what is left in the earth."),
    ]:
        room(key, name, description, "dungeon", "mine")
    link("mine_entrance", "down", "mine_lift")
    link("mine_lift", "north", "mine_junction")
    link("mine_junction", "east", "mine_office")
    link("mine_junction", "west", "mine_gallery")
    link("mine_junction", "north", "mine_pumps")
    link("mine_pumps", "east", "mine_seam")
    link("mine_office", "north", "mine_seam")

    for key, name, description in [
        ("grotto_steps", "Tideglass Steps", "Lighthouse steps end above the high-water mark. Up returns to the headland; north leads into luminous caves."),
        ("grotto_pool", "Mirror Pool", "A dry ledge circles a pool of stars. Shells gleam east, a tidal chart hangs west, and the lens chamber lies north."),
        ("grotto_shells", "Shell Archive", "Shell mosaics preserve a keeper's lesson: The MOON draws the water. The TIDE carries the vessel. The BEACON brings it home."),
        ("grotto_chart", "Tidal Chart Room", "A chart marks a safe path above every tide. A note says: Align the lens in the order of the keeper's lesson; a mistake begins the lesson again."),
        ("grotto_lens", "Tideglass Lens Chamber", "A forgotten lens is ringed by symbols of moon, tide, and beacon. Use align <symbol> to restore its light."),
        ("grotto_window", "Sea Window", "Beyond a stone balcony, waves glow with tiny blue lights. The lens chamber lies west; the shell archive is south."),
    ]:
        room(key, name, description, "dungeon", "grotto")
    link("lighthouse", "down", "grotto_steps")
    link("grotto_steps", "north", "grotto_pool")
    link("grotto_pool", "east", "grotto_shells")
    link("grotto_pool", "west", "grotto_chart")
    link("grotto_pool", "north", "grotto_lens")
    link("grotto_lens", "east", "grotto_window")
    link("grotto_shells", "north", "grotto_window")

    monster("crypt_sentinel", "crypt_choir", "Bronze Sentinel", "A hollow bronze guardian raises a chipped ceremonial blade.", health=24, attack=6, defense=1, gold=8, experience=15)
    monster("crypt_wraith", "crypt_ossuary", "Lantern Wraith", "Cold blue light gathers around an ancient keeper's shadow.", health=36, attack=8, defense=2, gold=12, experience=25)
    monster("mine_rat", "mine_gallery", "Tunnel Rat", "A large gray rat guards a nest of stolen copper wire.", health=18, attack=4, gold=5, experience=10, respawn=120)
    monster("mine_golem", "mine_seam", "Quartz Golem", "A slow stone construct grinds crystal fists together.", health=45, attack=10, defense=3, gold=18, experience=35)
    monster("grotto_crab", "grotto_shells", "Glassback Crab", "A translucent shell protects a pair of snapping claws.", health=28, attack=7, defense=2, gold=10, experience=20)
    monster("grotto_eel", "grotto_window", "Storm Eel", "A ribbon of electric blue coils over the wet stone.", health=38, attack=9, defense=1, gold=15, experience=30)

build()
