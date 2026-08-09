package base

// Key builders for the base profile's consumer vocabulary (§§4–6).
// Identifiers never contain dots (core §2 — deliberately, so dotted
// keys parse unambiguously), which is what makes consume.go's
// segment parsing sound.

func RenderGainKey(id string) string        { return EntityKey(id, "render.gain") }
func RenderAttenuationKey(id string) string { return EntityKey(id, "render.attenuation") }
func RenderSizeKey(id string) string        { return EntityKey(id, "render.size") }
func RenderDirectivityKey(id string) string { return EntityKey(id, "render.directivity") }

func HeardInKey(id, channel string) string { return EntityKey(id, "heard-in."+channel) }
func HearsKey(id, channel string) string   { return EntityKey(id, "hears."+channel) }

func PersonalMuteKey(owner, other string) string { return EntityKey(owner, "mute."+other) }
func PersonalSoloKey(owner, other string) string { return EntityKey(owner, "solo."+other) }

func DofKey(id string) string   { return EntityKey(id, "dof") }
func FrameKey(id string) string { return EntityKey(id, "frame") }

func SpaceEntityMutedKey(id string) string { return "lasa.space.entity.muted." + id }
func ClientBlockedKey(id string) string    { return "lasa.space.client.blocked." + id }
func RoleBlockedKey(role string) string    { return "lasa.space.role.blocked." + role }

func ChannelMutedKey(ch string) string       { return "lasa.space.channel.muted." + ch }
func ChannelGainKey(ch string) string        { return "lasa.space.channel.gain." + ch }
func ChannelAttenuationKey(ch string) string { return "lasa.space.channel.attenuation." + ch }
