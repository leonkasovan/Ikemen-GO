#if __VERSION__ >= 450
layout(binding = 0) uniform UniformBufferObject {
mat4 modelview, projection;
};
layout(location = 0) in vec2 position;
layout(location = 1) in vec2 uv;
layout(location = 2) uniform vec4 uvRect;
layout(location = 3) uniform int useUV;
layout(location = 0) out vec2 texcoord;
#else
#if __VERSION__ >= 130
#define COMPAT_VARYING out
#define COMPAT_ATTRIBUTE in
#define COMPAT_TEXTURE texture
#else
#define COMPAT_VARYING varying 
#define COMPAT_ATTRIBUTE attribute 
#define COMPAT_TEXTURE texture2D
#endif

uniform mat4 modelview, projection;

COMPAT_ATTRIBUTE vec2 position;
COMPAT_ATTRIBUTE vec2 uv;
uniform vec4 uvRect;
uniform int useUV;
COMPAT_VARYING vec2 texcoord;
#endif

void main(void) {
	if (useUV == 1) {
		// uvRect = (u1, v1, u2, v2) - remap 0..1 uv into rect
		texcoord = vec2(uvRect.x + uv.x * (uvRect.z - uvRect.x), uvRect.y + uv.y * (uvRect.w - uvRect.y));
	} else {
		texcoord = uv;
	}
	gl_Position = projection * (modelview * vec4(position, 0.0, 1.0));
	#if __VERSION__ >= 450
	gl_Position.y = -gl_Position.y;
	#endif
}
