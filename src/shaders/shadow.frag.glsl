struct Light
{
    vec3 direction;
    float range;

    vec3 color;
    float intensity;

    vec3 position;
    float innerConeCos;

    float outerConeCos;
    int type;

    float shadowBias;
    float shadowMapFar;
};
#if __VERSION__ >= 450
#define COMPAT_TEXTURE texture
layout (constant_id = 3) const bool useTexture = false;
layout(binding = 0) uniform EnvironmentUniform {
	layout(offset = 1536) Light lights[4];
};
layout(binding = 1) uniform MaterialUniform {
	mat3 texTransform;
	vec4 baseColorFactor;
	float ambientOcclusionStrength;
	float alphaThreshold;
	bool enableAlpha;
};
layout(binding = 5) uniform sampler2D tex;
layout(location = 0) in vec4 fragPos;
layout(location = 1) in float vColor;
layout(location = 2) in vec2 texcoord;
layout(location = 3) in flat int lightIndex;
#else
#if __VERSION__ >= 130
#define COMPAT_VARYING in
#define COMPAT_TEXTURE texture
#else
#define COMPAT_VARYING varying
#define COMPAT_TEXTURE texture2D
#endif
uniform sampler2D tex;
uniform mat3 texTransform;
uniform bool enableAlpha;
uniform bool useTexture;
uniform float alphaThreshold;
uniform vec4 baseColorFactor;
uniform Light lights[4];
uniform int lightIndex;
COMPAT_VARYING vec4 fragPos;
COMPAT_VARYING float vColor;
COMPAT_VARYING vec2 texcoord;
#endif

const int LightType_None = 0;
const int LightType_Directional = 1;
const int LightType_Point = 2;
const int LightType_Spot = 3;

void main()
{
    vec4 color = baseColorFactor;
    if(useTexture){
        color = color * COMPAT_TEXTURE(tex, vec2(texTransform*vec3(texcoord,1.0)));
    }
    color.a *= vColor;
    
    // Fixed: Use proper float comparison and separate conditions
    bool shouldDiscard = false;
    if(enableAlpha && color.a <= 0.0) {
        shouldDiscard = true;
    } else if(color.a < alphaThreshold) {
        shouldDiscard = true;
    }
    
    if(shouldDiscard) {
        discard;
    }
    
    int index = int(lightIndex);
    if(lights[index].type != LightType_Directional){
        float lightDistance = length(fragPos.xyz - lights[index].position);
    
        lightDistance = lightDistance / lights[index].shadowMapFar;
        
        gl_FragDepth = lightDistance;
    }else{
        gl_FragDepth = gl_FragCoord.z/gl_FragCoord.w;
    }
}
