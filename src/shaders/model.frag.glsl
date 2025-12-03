#if __VERSION__ >= 450
#define COMPAT_TEXTURE texture
#define COMPAT_TEXTURE_CUBE texture
#define COMPAT_TEXTURE_CUBE_LOD textureLod
#define COMPAT_SHADOW_MAP_TEXTURE(index) texture(shadowCubeMap,vec4(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0),index)).r
#define COMPAT_SHADOW_CUBE_MAP_TEXTURE(index) texture(shadowCubeMap,vec4(xyz,index)).r
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
layout(binding = 0) uniform EnvironmentUniform {
	layout(offset = 384) Light lights[4];
	layout(offset = 640) mat3 environmentRotation;
	layout(offset = 688) vec3 cameraPosition;
	layout(offset = 700) float environmentIntensity;
	layout(offset = 704) int mipCount;
};
layout(binding = 1) uniform MaterialUniform {
	mat3 texTransform,normalMapTransform,metallicRoughnessMapTransform,ambientOcclusionMapTransform,emissionMapTransform;
	vec4 baseColorFactor;
	vec3 emission;
	vec2 metallicRoughness;
	float ambientOcclusionStrength;
	float alphaThreshold;

	bool unlit;
	bool enableAlpha;
};
layout(binding = 2) uniform UniformBufferObject {
	layout(offset = 192) float meshOutline;
	layout(offset = 196) float gray;
	layout(offset = 200) float hue;
	layout(offset = 208) vec3 add;
	layout(offset = 224) vec3 mult;

};
layout (constant_id = 6) const bool useTexture = false;
layout (constant_id = 7) const bool useNormalMap = false;
layout (constant_id = 8) const bool useMetallicRoughnessMap = false;
layout (constant_id = 9) const bool useEmissionMap = false;
layout (constant_id = 10) const bool neg = false;
layout (constant_id = 11) const bool useShadowMap = false;

layout(binding = 5) uniform samplerCube lambertianEnvSampler;
layout(binding = 6) uniform samplerCube GGXEnvSampler;
layout(binding = 7) uniform sampler2D GGXLUT;

layout(binding = 8) uniform sampler2D tex;
layout(binding = 9) uniform sampler2D normalMap;
layout(binding = 10) uniform sampler2D metallicRoughnessMap;
layout(binding = 11) uniform sampler2D ambientOcclusionMap;
layout(binding = 12) uniform sampler2D emissionMap;
layout(binding = 13) uniform samplerCubeArray shadowCubeMap;

layout(location = 0) in vec3 normal;
layout(location = 1) in vec3 tangent;
layout(location = 2) in vec3 bitangent;
layout(location = 3) in vec2 texcoord;
layout(location = 4) in vec4 vColor;
layout(location = 5) in vec3 worldSpacePos;
layout(location = 6) in vec4 lightSpacePos[4];
layout(location = 0) out vec4 FragColor;
#else
#if __VERSION__ >= 130
// For desktop OpenGL 130+, use the cube map array extension
#if !defined(GL_ES) && defined(GL_ARB_texture_cube_map_array)
#extension GL_ARB_texture_cube_map_array : enable
#endif
#define COMPAT_VARYING in
#define COMPAT_TEXTURE texture
#define COMPAT_TEXTURE_CUBE texture
#define COMPAT_TEXTURE_CUBE_LOD textureLod
#ifdef ENABLE_SHADOW
#if defined(GL_ES) || __VERSION__ < 400
// For OpenGL ES, use separate samplerCube arrays instead of samplerCubeArray
#define COMPAT_SHADOW_MAP_TEXTURE(index) (index == 0 ? texture(shadowCubeMaps[0], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r : index == 1 ? texture(shadowCubeMaps[1], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r : index == 2 ? texture(shadowCubeMaps[2], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r : texture(shadowCubeMaps[3], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r)
#define COMPAT_SHADOW_CUBE_MAP_TEXTURE(index) (index == 0 ? texture(shadowCubeMaps[0], xyz).r : index == 1 ? texture(shadowCubeMaps[1], xyz).r : index == 2 ? texture(shadowCubeMaps[2], xyz).r : texture(shadowCubeMaps[3], xyz).r)
#else
#define COMPAT_SHADOW_MAP_TEXTURE(index) texture(shadowCubeMap,vec4(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0),index)).r
#define COMPAT_SHADOW_CUBE_MAP_TEXTURE(index) texture(shadowCubeMap,vec4(xyz,index)).r
#endif
#endif
#else
// OpenGL ES or legacy OpenGL
#if __VERSION__ >= 300
// GLSL ES 300+
#extension GL_EXT_shader_texture_lod : enable
#define COMPAT_VARYING in
#define FragColor gl_FragColor
#define COMPAT_TEXTURE texture
#define COMPAT_TEXTURE_CUBE texture
#define COMPAT_TEXTURE_CUBE_LOD textureLod
#ifdef ENABLE_SHADOW
#define COMPAT_SHADOW_MAP_TEXTURE(index) (index == 0 ? texture(shadowCubeMaps[0], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r : index == 1 ? texture(shadowCubeMaps[1], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r : index == 2 ? texture(shadowCubeMaps[2], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r : texture(shadowCubeMaps[3], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r)
#define COMPAT_SHADOW_CUBE_MAP_TEXTURE(index) (index == 0 ? texture(shadowCubeMaps[0], xyz).r : index == 1 ? texture(shadowCubeMaps[1], xyz).r : index == 2 ? texture(shadowCubeMaps[2], xyz).r : texture(shadowCubeMaps[3], xyz).r)
#endif
#else
// Legacy GLSL
#extension GL_EXT_gpu_shader4 : enable
#extension GL_ARB_shader_texture_lod : enable
#define COMPAT_VARYING varying
#define FragColor gl_FragColor
#define COMPAT_TEXTURE texture2D
#define COMPAT_TEXTURE_CUBE textureCube
#define COMPAT_TEXTURE_CUBE_LOD textureCubeLod
#ifdef ENABLE_SHADOW
#define COMPAT_SHADOW_MAP_TEXTURE(index) textureCube(shadowCubeMaps[index], vec3(1.0, -(xy.y*2.0-1.0),-(xy.x*2.0-1.0))).r
#define COMPAT_SHADOW_CUBE_MAP_TEXTURE(index) textureCube(shadowCubeMaps[index], xyz).r
#endif
#endif
#endif

// Uniform declarations must come after version directives but before struct definitions
#ifdef ENABLE_SHADOW
#if defined(GL_ES) || __VERSION__ < 400
// For OpenGL ES or older desktop OpenGL, use separate samplerCube arrays
uniform samplerCube shadowCubeMaps[4];
#else
// For modern desktop OpenGL, use samplerCubeArray
uniform samplerCubeArray shadowCubeMap;
#endif
#endif

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

// Now declare all uniforms
uniform sampler2D tex;
uniform mat3 texTransform;
uniform sampler2D normalMap;
uniform mat3 normalMapTransform;
uniform sampler2D metallicRoughnessMap;
uniform mat3 metallicRoughnessMapTransform;
uniform sampler2D ambientOcclusionMap;
uniform mat3 ambientOcclusionMapTransform;
uniform sampler2D emissionMap;
uniform mat3 emissionMapTransform;
uniform samplerCube lambertianEnvSampler;
uniform samplerCube GGXEnvSampler;
uniform sampler2D GGXLUT;
uniform float environmentIntensity;
uniform mat3 environmentRotation;
uniform int mipCount;

uniform vec3 cameraPosition;

uniform vec4 baseColorFactor;
uniform vec2 metallicRoughness;
uniform float ambientOcclusionStrength;
uniform vec3 emission;
uniform bool unlit;

uniform Light lights[4];

uniform vec3 add, mult;
uniform float gray, hue;
uniform bool useTexture;
uniform bool useNormalMap;
uniform bool useMetallicRoughnessMap;
uniform bool useEmissionMap;
uniform bool neg;
uniform bool enableAlpha;
uniform float alphaThreshold;
uniform float meshOutline;

COMPAT_VARYING vec2 texcoord;
COMPAT_VARYING vec4 vColor;
COMPAT_VARYING vec3 normal;
COMPAT_VARYING vec3 tangent;
COMPAT_VARYING vec3 bitangent;
COMPAT_VARYING vec3 worldSpacePos;
COMPAT_VARYING vec4 lightSpacePos[4];

// Output declaration for GLSL 130+
#if __VERSION__ >= 130 && !defined(GL_ES)
out vec4 FragColor;
#elif __VERSION__ >= 300
// GLSL ES 300+ uses out
out vec4 FragColor;
#endif

const bool useShadowMap = true;
#endif


const float PI = 3.14159265358979;
const int LightType_Directional = 0;
const int LightType_Point = 1;
const int LightType_Spot = 2;

float clampedDot(vec3 x, vec3 y)
{
    return clamp(dot(x, y), 0.0, 1.0);
}

float DirectionalLightShadowCalculation(int index, vec4 lightSpacePos,float NdotL,float shadowBias)
{
    #ifdef ENABLE_SHADOW
    if(!useShadowMap){
        return 1.0;
    }
    // perform perspective divide
    vec3 projCoords = lightSpacePos.xyz / lightSpacePos.w;
    // transform to [0,1] range
    projCoords = projCoords * 0.5 + 0.5;
    // get closest depth value from light's perspective (using [0,1] range fragPosLight as coords)
    float epsilon = 1.0 / 1024.0;
    vec2 xy = vec2(clamp(projCoords.x,epsilon,1.0-epsilon),clamp(projCoords.y,epsilon,1.0-epsilon));
    float closestDepth = COMPAT_SHADOW_MAP_TEXTURE(index);
    // get depth of current fragment from light's perspective
    float currentDepth = projCoords.z;
    // check whether current frag pos is in shadow
    float bias = shadowBias*tan(acos(NdotL));
    float shadow = closestDepth-currentDepth > -bias  ? 1.0 : 0.0;
    #else
    float shadow = 1.0;
    #endif
    return shadow;
}

float SpotLightShadowCalculation(int index, vec3 pointToLight, vec4 lightSpacePos,float NdotL,float farPlane,float shadowBias)
{
    #ifdef ENABLE_SHADOW
    if(!useShadowMap){
        return 1.0;
    }
    float epsilon = 1.0 / 1024.0;
    vec2 xy = vec2(clamp(lightSpacePos.x,epsilon,1.0-epsilon),clamp(lightSpacePos.y,epsilon,1.0-epsilon));
    float closestDepth = COMPAT_SHADOW_MAP_TEXTURE(index);
    // it is currently in linear range between [0,1]. Re-transform back to original value
    closestDepth *= farPlane;
    // get depth of current fragment from light's perspective
    float currentDepth = length(pointToLight);
    float bias = shadowBias*tan(acos(NdotL));
    float shadow = currentDepth-closestDepth < bias  ? 1.0 : 0.0;
    #else
    float shadow = 1.0;
    #endif
    return shadow;
}

float PointLightShadowCalculation(int index, vec3 pointToLight,float NdotL,float farPlane,float shadowBias)
{
    #ifdef ENABLE_SHADOW
    if(!useShadowMap){
        return 1.0;
    }
    vec3 xyz = -pointToLight;
    float closestDepth = COMPAT_SHADOW_CUBE_MAP_TEXTURE(index);
    // it is currently in linear range between [0,1]. Re-transform back to original value
    closestDepth *= farPlane;
    // now get current linear depth as the length between the fragment and light position
    float currentDepth = length(pointToLight);

    float bias = shadowBias*tan(acos(NdotL));

    float shadow = currentDepth-closestDepth < bias  ? 1.0 : 0.0;

    #else
    float shadow = 1.0;
    #endif
    return shadow;
}

vec3 getNormal()
{
    vec2 uv_dx = dFdx(texcoord);
    vec2 uv_dy = dFdy(texcoord);
    if (length(uv_dx) <= 1e-2) {
      uv_dx = vec2(1.0, 0.0);
    }

    if (length(uv_dy) <= 1e-2) {
      uv_dy = vec2(0.0, 1.0);
    }
    vec3 t_ = (uv_dy.t * dFdx(worldSpacePos) - uv_dx.t * dFdy(worldSpacePos)) /
        (uv_dx.s * uv_dy.t - uv_dy.s * uv_dx.t);
    vec3 n, t, b, ng;
    if(normal.x+normal.y+normal.z != 0.0){
        if(tangent.x+tangent.y+tangent.z != 0.0){
            t = normalize(tangent);
            b = normalize(bitangent);
            ng = normalize(normal);
        }else{
            ng = normalize(normal);
            t = normalize(t_ - ng * dot(ng, t_));
            b = cross(ng, t);
        }
    }else{
        ng = normalize(cross(dFdx(worldSpacePos), dFdy(worldSpacePos)));
        t = normalize(t_ - ng * dot(ng, t_));
        b = cross(ng, t);
    }
    if (gl_FrontFacing == false)
    {
        t *= -1.0;
        b *= -1.0;
        ng *= -1.0;
    }
    if(useNormalMap){
        return normalize(mat3(t, b, ng) * normalize(COMPAT_TEXTURE(normalMap, vec2(normalMapTransform*vec3(texcoord,1.0))).xyz * 2.0 - vec3(1.0)));
    }else{
        return ng;
    }
}

// https://github.com/KhronosGroup/glTF/blob/master/extensions/2.0/Khronos/KHR_lights_punctual/README.md#range-property
float getRangeAttenuation(float range, float distance)
{
    if (range <= 0.0)
    {
        // negative range means unlimited
        return 1.0 / pow(distance, 2.0);
    }
    return max(min(1.0 - pow(distance / range, 4.0), 1.0), 0.0) / pow(distance, 2.0);
}


// https://github.com/KhronosGroup/glTF/blob/master/extensions/2.0/Khronos/KHR_lights_punctual/README.md#inner-and-outer-cone-angles
float getSpotAttenuation(vec3 pointToLight, vec3 spotDirection, float outerConeCos, float innerConeCos)
{
    float actualCos = dot(normalize(spotDirection), normalize(-pointToLight));
    if (actualCos > outerConeCos)
    {
        if (actualCos < innerConeCos)
        {
            float angularAttenuation = (actualCos - outerConeCos) / (innerConeCos - outerConeCos);
            return angularAttenuation * angularAttenuation;
        }
        return 1.0;
    }
    return 0.0;
}
vec3 getLighIntensity(Light light, vec3 pointToLight)
{
    float rangeAttenuation = 1.0;
    float spotAttenuation = 1.0;

    if (light.type != LightType_Directional)
    {
        rangeAttenuation = getRangeAttenuation(light.range, length(pointToLight));
    }
    if (light.type == LightType_Spot)
    {
        spotAttenuation = getSpotAttenuation(pointToLight, light.direction, light.outerConeCos, light.innerConeCos);
    }

    return rangeAttenuation * spotAttenuation * light.intensity * light.color;
}
vec3 F_Schlick(vec3 f0, vec3 f90, float VdotH)
{
    return f0 + (f90 - f0) * pow(clamp(1.0 - VdotH, 0.0, 1.0), 5.0);
}
// Smith Joint GGX
// Note: Vis = G / (4 * NdotL * NdotV)
// see Eric Heitz. 2014. Understanding the Masking-Shadowing Function in Microfacet-Based BRDFs. Journal of Computer Graphics Techniques, 3
// see Real-Time Rendering. Page 331 to 336.
// see https://google.github.io/filament/Filament.md.html#materialsystem/specularbrdf/geometricshadowing(specularg)
float V_GGX(float NdotL, float NdotV, float alphaRoughness)
{
    float alphaRoughnessSq = alphaRoughness * alphaRoughness;

    float GGXV = NdotL * sqrt(NdotV * NdotV * (1.0 - alphaRoughnessSq) + alphaRoughnessSq);
    float GGXL = NdotV * sqrt(NdotL * NdotL * (1.0 - alphaRoughnessSq) + alphaRoughnessSq);

    float GGX = GGXV + GGXL;
    if (GGX > 0.0)
    {
        return 0.5 / GGX;
    }
    return 0.0;
}

// The following equation(s) model the distribution of microfacet normals across the area being drawn (aka D())
// Implementation from "Average Irregularity Representation of a Roughened Surface for Ray Reflection" by T. S. Trowbridge, and K. P. Reitz
// Follows the distribution function recommended in the SIGGRAPH 2013 course notes from EPIC Games [1], Equation 3.
float D_GGX(float NdotH, float alphaRoughness)
{
    float alphaRoughnessSq = alphaRoughness * alphaRoughness;
    float f = (NdotH * NdotH) * (alphaRoughnessSq - 1.0) + 1.0;
    return alphaRoughnessSq / (PI * f * f);
}
vec3 BRDF_lambertian(vec3 f0, vec3 f90, vec3 diffuseColor, float specularWeight, float VdotH)
{
    // see https://seblagarde.wordpress.com/2012/01/08/pi-or-not-to-pi-in-game-lighting-equation/
    return (1.0 - specularWeight * F_Schlick(f0, f90, VdotH)) * (diffuseColor / PI);
}
vec3 BRDF_specularGGX(vec3 f0, vec3 f90, float alphaRoughness, float specularWeight, float VdotH, float NdotL, float NdotV, float NdotH)
{
    vec3 F = F_Schlick(f0, f90, VdotH);
    float Vis = V_GGX(NdotL, NdotV, alphaRoughness);
    float D = D_GGX(NdotH, alphaRoughness);

    return specularWeight * F * Vis * D;
}
vec3 getDiffuseLight(vec3 n)
{
    return COMPAT_TEXTURE_CUBE(lambertianEnvSampler, environmentRotation * n).rgb * environmentIntensity;
}
vec4 getSpecularSample(vec3 reflection, float lod)
{
    return COMPAT_TEXTURE_CUBE_LOD(GGXEnvSampler, environmentRotation * reflection, lod) * environmentIntensity;
}
vec3 getIBLGGXFresnel(vec3 n, vec3 v, float roughness, vec3 F0, float specularWeight)
{
    // see https://bruop.github.io/ibl/#single_scattering_results at Single Scattering Results
    // Roughness dependent fresnel, from Fdez-Aguera
    float NdotV = clampedDot(n, v);
    vec2 brdfSamplePoint = clamp(vec2(NdotV, roughness), vec2(0.0, 0.0), vec2(1.0, 1.0));
    vec2 f_ab = COMPAT_TEXTURE(GGXLUT, brdfSamplePoint).rg;
    vec3 Fr = max(vec3(1.0 - roughness), F0) - F0;
    vec3 k_S = F0 + Fr * pow(1.0 - NdotV, 5.0);
    vec3 FssEss = specularWeight * (k_S * f_ab.x + f_ab.y);

    // Multiple scattering, from Fdez-Aguera
    float Ems = (1.0 - (f_ab.x + f_ab.y));
    vec3 F_avg = specularWeight * (F0 + (1.0 - F0) / 21.0);
    vec3 FmsEms = Ems * FssEss * F_avg / (1.0 - F_avg * Ems);

    return FssEss + FmsEms;
}
vec3 getIBLRadianceGGX(vec3 n, vec3 v, float roughness)
{
    float NdotV = clampedDot(n, v);
    float lod = roughness * float(mipCount - 1);
    vec3 reflection = normalize(reflect(-v, n));
    vec4 specularSample = getSpecularSample(reflection, lod);

    vec3 specularLight = specularSample.rgb;

    return specularLight;
}
vec3 ibl(vec3 n, vec3 v, float metallic, float roughness, vec3 albedo)
{
    vec3 f_diffuse = getDiffuseLight(n) * albedo.rgb;
    vec3 f_specular_metal = getIBLRadianceGGX(n, v, roughness);
    vec3 f_specular_dielectric = f_specular_metal;

    // Fixed calls with proper float literals
    vec3 f_metal_fresnel_ibl = getIBLGGXFresnel(n, v, roughness, albedo.rgb, 1.0);
    vec3 f_dielectric_fresnel_ibl = getIBLGGXFresnel(n, v, roughness, vec3(0.04), 1.0);

    vec3 f_metal_brdf_ibl = f_metal_fresnel_ibl * f_specular_metal;
    vec3 f_dielectric_brdf_ibl = f_diffuse * (1.0 - f_dielectric_fresnel_ibl) + f_specular_dielectric * f_dielectric_fresnel_ibl;

    vec3 color = f_dielectric_brdf_ibl * (1.0 - metallic) + f_metal_brdf_ibl * metallic;
    return color;
}
vec3 pbr(vec3 worldSpacePos,vec3 v,vec3 n,vec3 albedo,float metallic,float roughness,float ao){
	vec3 f0 = vec3(0.04)+(albedo-vec3(0.04))*metallic;
    vec3 f90 = vec3(1.0);
    float ior = 1.5;
    float specularWeight = 1.0;
    vec3 f_specular = vec3(0.0);
    vec3 f_diffuse = vec3(0.0);
    vec3 c_diff = albedo*(1.0-metallic);

	for(int i = 0; i < 4; ++i)
    {
        if(lights[i].color.r+lights[i].color.g+lights[i].color.b > 0.0){
            vec3 pointToLight = vec3(0.0);
            if(lights[i].type == LightType_Directional){
                pointToLight = -lights[i].direction;
            }else{
                pointToLight = lights[i].position - worldSpacePos;
            }
            vec3 l = normalize(pointToLight);
            vec3 h = normalize(l + v);
            float NdotL = clampedDot(n, l);
            float NdotV = clampedDot(n, v);
            float NdotH = clampedDot(n, h);
            float VdotH = clampedDot(v, h);
            if (NdotL > 0.0 || NdotV > 0.0){
                vec3 intensity = getLighIntensity(lights[i], pointToLight);
                vec3 l_diffuse = vec3(0.0);
                vec3 l_specular = vec3(0.0);
                l_diffuse += intensity * NdotL *  BRDF_lambertian(f0, f90, c_diff, specularWeight, VdotH);
                l_specular += intensity * NdotL * BRDF_specularGGX(f0, f90, roughness*roughness, specularWeight, VdotH, NdotL, NdotV, NdotH);
                float shadow = 1.0;
                if(lights[i].type == LightType_Point){
                    shadow = PointLightShadowCalculation(i,pointToLight,NdotL,lights[i].shadowMapFar,lights[i].shadowBias);
                }else if(lights[i].type == LightType_Directional){
                    shadow = DirectionalLightShadowCalculation(i,lightSpacePos[i],NdotL,lights[i].shadowBias);
                }else{
                    shadow = SpotLightShadowCalculation(i,pointToLight,lightSpacePos[i],NdotL,lights[i].shadowMapFar,lights[i].shadowBias);
                }
                f_diffuse += l_diffuse * shadow;
                f_specular += l_specular * shadow;
            }
        }
    }
    vec3 f_ibl = vec3(0.0);
    if(environmentIntensity > 0.0){
        f_ibl = ibl(n,v,metallic,roughness,albedo);
    }
    //vec3 color = clamp(f_diffuse+f_specular+ao*f_ibl,0,1);
    vec3 color = f_diffuse+f_specular+ao*f_ibl;

    //color = color / (color + vec3(1.0));
    //color = pow(color, vec3(1.0/2.2));
	return color;
}
vec3 hue_shift(vec3 color, float dhue) {
	float s = sin(dhue);
	float c = cos(dhue);
	return (color * c) + (color * s) * mat3(
		vec3(0.167444, 0.329213, -0.496657),
		vec3(-0.327948, 0.035669, 0.292279),
		vec3(1.250268, -1.047561, -0.202707)
	) + dot(vec3(0.299, 0.587, 0.114), color) * (1.0 - c);
}

void main(void) {
    FragColor = vec4(1.0);
	if(useTexture){
		FragColor = COMPAT_TEXTURE(tex, vec2(texTransform*vec3(texcoord,1.0)));
        FragColor.rgb = pow(FragColor.rgb,vec3(2.2));
	}
    FragColor *= baseColorFactor;
	FragColor *= vColor;
    if(meshOutline > 0.0) {
        FragColor.rgb = vec3(0.0,0.0,0.0);
    }else if(!unlit){
        vec3 normalF = normal;
        if(useNormalMap){
            normalF = getNormal();
        }
        vec2 metallicRoughnessF = metallicRoughness;
        if(useMetallicRoughnessMap){
            metallicRoughnessF = COMPAT_TEXTURE(metallicRoughnessMap, vec2(metallicRoughnessMapTransform*vec3(texcoord,1.0))).bg;
        }
        float ambientOcclusion = 1.0;
        if(ambientOcclusionStrength > 0.0){
            ambientOcclusion = 1.0+ambientOcclusionStrength*(COMPAT_TEXTURE(ambientOcclusionMap, vec2(ambientOcclusionMapTransform*vec3(texcoord,1.0))).r-1.0);
        }
        FragColor.rgb = pbr(worldSpacePos,normalize(cameraPosition - worldSpacePos),normalize(normalF),FragColor.rgb,metallicRoughnessF[0],metallicRoughnessF[1],ambientOcclusion);
        if(useEmissionMap){
            FragColor.rgb += emission * pow(COMPAT_TEXTURE(emissionMap, vec2(emissionMapTransform*vec3(texcoord,1.0))).rgb,vec3(2.2));
        }else{
            FragColor.rgb += emission;
        }
    }
    FragColor.rgb = pow(FragColor.rgb, vec3(1.0/2.2));
    FragColor.rgb *= vColor.a;
	if(!enableAlpha){
		if(FragColor.a < alphaThreshold){
			discard;
		}else{
			FragColor.a = 1.0;
		}
	}else if(FragColor.a<=0.0){
		discard;
	}
	vec3 neg_base = vec3(1.0);
	neg_base *= FragColor.a;
	if (hue != 0.0) {
		FragColor.rgb = hue_shift(FragColor.rgb,hue);
	}
	if (neg) FragColor.rgb = neg_base - FragColor.rgb;
	FragColor.rgb = mix(FragColor.rgb, vec3((FragColor.r + FragColor.g + FragColor.b) / 3.0), gray) + add*FragColor.a;
	FragColor.rgb *= mult;
}
