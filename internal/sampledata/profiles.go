package sampledata

// Profils de véhicule du jeu de démo (ADR 0036), par clé logique de BOM. Ils ne font pas partie du Dataset
// (ni de son empreinte) : comme les tables partagées du coût complet, ils sont posés au seed sans écraser un
// profil déjà choisi par l'utilisateur. Poids : coût, CO₂, délai, masse, qualité fournisseur (somme 100).

// VehicleProfile : préréglage et poids d'un profil de démo.
type VehicleProfile struct {
	Preset                             string
	Cost, CO2, LeadTime, Mass, Quality int
}

// DemoProfiles : la Multipla, monospace familial, en profil « familiale ».
var DemoProfiles = map[string]VehicleProfile{
	"bom.multipla": {Preset: "family", Cost: 40, CO2: 25, LeadTime: 20, Mass: 5, Quality: 10},
}
