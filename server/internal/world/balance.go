package world

import "juegito/server/internal/protocol"

// Argentum's class and race tables, ported from Balance.dat.
//
// These numbers are the game's balance, tuned over two decades. They are copied
// rather than invented, and the file they come from is right there if any of
// them ever needs checking.

// Class is an Argentum character class.
type Class int

const (
	Guerrero Class = iota
	Cazador
	Paladin
	Bandido
	Asesino
	Pirata
	Ladron
	Clerigo
	Bardo
	Mago
	Druida
	Trabajador
)

var className = map[Class]string{
	Guerrero: "Guerrero", Cazador: "Cazador", Paladin: "Paladín", Bandido: "Bandido",
	Asesino: "Asesino", Pirata: "Pirata", Ladron: "Ladrón", Clerigo: "Clérigo",
	Bardo: "Bardo", Mago: "Mago", Druida: "Druida", Trabajador: "Trabajador",
}

func (c Class) String() string { return className[c] }

// classMods holds one class's columns of Balance.dat.
type classMods struct {
	Evasion         float64
	AtaqueArmas     float64
	AtaqueWrestling float64
	DanoArmas       float64
	DanoWrestling   float64
	Escudo          float64
	Vida            float64
}

// classModifiers is Balance.dat's [MOD*] sections, transposed so one class is
// one row. A mage evades at 0.4 and a pirate at 1.25 — that spread is the
// character of the game and none of it is arbitrary.
var classModifiers = map[Class]classMods{
	Guerrero:   {Evasion: 1.00, AtaqueArmas: 1.00, AtaqueWrestling: 0.60, DanoArmas: 1.100, DanoWrestling: 0.40, Escudo: 1.00, Vida: 10.0},
	Cazador:    {Evasion: 0.90, AtaqueArmas: 0.80, AtaqueWrestling: 0.50, DanoArmas: 0.900, DanoWrestling: 0.40, Escudo: 0.80, Vida: 9.5},
	Paladin:    {Evasion: 0.90, AtaqueArmas: 0.95, AtaqueWrestling: 0.40, DanoArmas: 0.925, DanoWrestling: 0.40, Escudo: 1.00, Vida: 9.5},
	Bandido:    {Evasion: 0.70, AtaqueArmas: 0.85, AtaqueWrestling: 0.95, DanoArmas: 0.850, DanoWrestling: 1.05, Escudo: 2.00, Vida: 9.5},
	Asesino:    {Evasion: 1.10, AtaqueArmas: 0.90, AtaqueWrestling: 0.40, DanoArmas: 0.900, DanoWrestling: 0.40, Escudo: 0.80, Vida: 8.5},
	Pirata:     {Evasion: 1.25, AtaqueArmas: 0.90, AtaqueWrestling: 0.50, DanoArmas: 0.950, DanoWrestling: 0.40, Escudo: 0.60, Vida: 9.5},
	Ladron:     {Evasion: 1.10, AtaqueArmas: 0.80, AtaqueWrestling: 0.80, DanoArmas: 0.750, DanoWrestling: 1.05, Escudo: 0.70, Vida: 10.0},
	Clerigo:    {Evasion: 0.80, AtaqueArmas: 0.85, AtaqueWrestling: 0.40, DanoArmas: 0.800, DanoWrestling: 0.40, Escudo: 0.85, Vida: 8.5},
	Bardo:      {Evasion: 1.075, AtaqueArmas: 0.70, AtaqueWrestling: 0.40, DanoArmas: 0.750, DanoWrestling: 0.40, Escudo: 0.80, Vida: 8.5},
	Mago:       {Evasion: 0.40, AtaqueArmas: 0.50, AtaqueWrestling: 0.30, DanoArmas: 0.500, DanoWrestling: 0.40, Escudo: 0.60, Vida: 7.5},
	Druida:     {Evasion: 0.75, AtaqueArmas: 0.65, AtaqueWrestling: 0.40, DanoArmas: 0.700, DanoWrestling: 0.40, Escudo: 0.75, Vida: 8.5},
	Trabajador: {Evasion: 0.80, AtaqueArmas: 0.80, AtaqueWrestling: 0.50, DanoArmas: 0.800, DanoWrestling: 0.40, Escudo: 0.70, Vida: 9.5},
}

// Race is an Argentum character race.
type Race int

const (
	Humano Race = iota
	Elfo
	Drow
	Enano
	Gnomo
)

var raceName = map[Race]string{
	Humano: "Humano", Elfo: "Elfo", Drow: "Elfo Oscuro", Enano: "Enano", Gnomo: "Gnomo",
}

func (r Race) String() string { return raceName[r] }

// Attributes are Argentum's five character attributes.
type Attributes struct {
	Fuerza       int
	Agilidad     int
	Inteligencia int
	Carisma      int
	Constitucion int
}

// raceModifiers is Balance.dat's [MODRAZA]: what each race adds to the rolled
// base attributes. A dwarf is +3 strength and -2 intelligence; a gnome the
// reverse.
var raceModifiers = map[Race]Attributes{
	Humano: {Fuerza: 1, Agilidad: 1, Inteligencia: 0, Carisma: 0, Constitucion: 2},
	Elfo:   {Fuerza: -1, Agilidad: 3, Inteligencia: 2, Carisma: 2, Constitucion: 1},
	Drow:   {Fuerza: 2, Agilidad: 3, Inteligencia: 2, Carisma: -3, Constitucion: 0},
	Enano:  {Fuerza: 3, Agilidad: 0, Inteligencia: -2, Carisma: -2, Constitucion: 3},
	Gnomo:  {Fuerza: -2, Agilidad: 3, Inteligencia: 4, Carisma: 1, Constitucion: 0},
}

// Skills are the handful of Argentum skills combat and casting actually read.
type Skills struct {
	Armas     int
	Wrestling int
	Tacticas  int
	Defensa   int
	// Magia gates casting: PuedeLanzar in the source refuses a spell whose
	// MinSkill exceeds it. It's a separate skill from Armas/Wrestling on
	// purpose — a fighter build and a caster build cost different points in
	// classic AO, even though this port skips the grind that would normally
	// separate them.
	Magia int
}

var allClasses = []Class{
	Guerrero, Cazador, Paladin, Bandido, Asesino, Pirata,
	Ladron, Clerigo, Bardo, Mago, Druida, Trabajador,
}

var allRaces = []Race{Humano, Elfo, Drow, Enano, Gnomo}

// maxLevel is where every character spawns.
//
// This project doesn't grind: nobody has a level 1 you'd ever see. 45 is the
// commonly cited cap for classic Argentum/Alkon, used here as a reasonable
// target for the level-scaling terms in combat and spells rather than as a
// number pulled from a grepped constant — Declares.bas loads STAT_MAXELV from
// server config rather than hardcoding it, so there's no single "the" value.
const maxLevel = 45

// baseAttribute is what every character starts from before its race adjusts
// it. Argentum rolls dice across a match's worth of characters; here, with
// everyone spawning at the level cap and nobody grinding, only the race
// modifiers create the spread between characters. 30 approximates a
// maxed-out roll under Argentum's usual attribute range (roughly 6-38).
const baseAttribute = 30

func rolledAttributes(race Race) Attributes {
	mod := raceModifiers[race]
	return Attributes{
		Fuerza:       baseAttribute + mod.Fuerza,
		Agilidad:     baseAttribute + mod.Agilidad,
		Inteligencia: baseAttribute + mod.Inteligencia,
		Carisma:      baseAttribute + mod.Carisma,
		Constitucion: baseAttribute + mod.Constitucion,
	}
}

// startingSkills gives everyone the maximum in every skill combat and casting
// read. Argentum's own scale tops out at 100, and the 31/61/91 breakpoints in
// poderAtaque assume a character can actually reach the top band.
var startingSkills = Skills{Armas: 100, Wrestling: 100, Tacticas: 100, Defensa: 100, Magia: 100}

// vitalsFor scales health by the class's MODVIDA column times maxLevel —
// Argentum's Vida is health-per-level, so a warrior's 10/level against a
// mage's 7.5/level compounds into a real gap at the cap: 450 HP vs 337.
func vitalsFor(class Class) protocol.Vitals {
	vitals := startingVitals
	vitals.Level = maxLevel
	vitals.MaxExp = 0 // there is no next level to progress toward
	vitals.MaxHP = int(classModifiers[class].Vida * maxLevel)
	vitals.HP = vitals.MaxHP
	return vitals
}
