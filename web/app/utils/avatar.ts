// import this only from DefaultAvatar.vue. Any other import puts the
// avatar library into the main bundle.
import { Avatar, Style } from "@dicebear/core"
import planets from "@dicebear/styles/planets.json"
import shadows from "@dicebear/styles/shadows.json"

export type AvatarKind =
	"user" | "organization" | "deleted-user" | "invited-user"

// the warm half of the planet colours the style ships
const PLANET_COLORS = ["e27a8c", "e37f64", "d88a40", "c1982a", "d67cb2"]
// how often each of the twelve star slots holds a star, in percent
const STAR_PROBABILITY = 25
const STAR_SCALE = 3
const SPARKLE_SCALE = 2.2
const MOON_SCALE = 1.6

const userStyle = new Style(shadows)
const organizationStyle = new Style(readablePlanets())
const cache = new Map<string, string>()

// the default picture of a user or an organization, as a data uri. The same
// seed always gives the same picture. Every deleted user shares one grey
// picture, and every invited user another, so those kinds take no seed. The
// picture carries no title: a screen reader ignores one inside an image,
// and reads the image's alt text
export function defaultAvatar(kind: AvatarKind, seed = ""): string {
	const key = `${kind}:${seed}`
	const cached = cache.get(key)
	if (cached) {
		return cached
	}

	const uri = draw(kind, seed).toDataUri()

	// the server keeps no cache. It would outlive the request and grow with
	// every seed ever drawn
	if (import.meta.client) {
		cache.set(key, uri)
	}

	return uri
}

function draw(kind: AvatarKind, seed: string) {
	if (kind === "user") {
		return new Avatar(userStyle, { seed })
	}

	if (kind === "deleted-user") {
		return new Avatar(userStyle, {
			seed: kind,
			backgroundColor: "#e5e5e5",
			inkColor: "#a3a3a3",
		})
	}

	if (kind === "invited-user") {
		return new Avatar(userStyle, {
			seed: kind,
			backgroundColor: "#e0e7ef",
			inkColor: "#94a3b8",
		})
	}

	// the seed picks between a still planet and the slowest animation
	return new Avatar(organizationStyle, {
		seed,
		animationVariant: ["none", "slowest"],
		planetColor: PLANET_COLORS,
		starVariant: ["large", "sparkle"],
		moonsVariant: ["one", "two"],
	})
}

// the planets style, redrawn for small tiles: fewer stars, and bigger stars
// and moons. At the size of a sidebar tile the style's own are too fine to
// see. The style has no option for size, so its definition is changed
function readablePlanets() {
	const definition = structuredClone(planets)
	const { star, moons } = definition.components

	star.probability = STAR_PROBABILITY
	scaleShapes(star.variants.large, STAR_SCALE)
	scaleShapes(star.variants.sparkle, SPARKLE_SCALE)
	scaleShapes(moons.variants.one, MOON_SCALE)
	scaleShapes(moons.variants.two, MOON_SCALE)

	return definition
}

// scaleShapes grows every circle and path below a node of a style
// definition. A circle grows through its radius. A star's sparkle is a path,
// which grows around the middle of the 10 by 10 box a star is drawn in
function scaleShapes(node: unknown, factor: number) {
	if (Array.isArray(node)) {
		node.forEach((child) => {
			scaleShapes(child, factor)
		})

		return
	}

	if (typeof node !== "object" || node === null) {
		return
	}

	const shape = node as {
		name?: unknown
		attributes?: Record<string, unknown>
	}

	if (shape.name === "circle" && shape.attributes) {
		shape.attributes.r = String(
			Math.round(Number(shape.attributes.r) * factor * 100) / 100,
		)
	}

	if (shape.name === "path" && shape.attributes) {
		shape.attributes.transform = `translate(5 5) scale(${factor}) translate(-5 -5)`
	}

	Object.values(node).forEach((child) => {
		scaleShapes(child, factor)
	})
}
