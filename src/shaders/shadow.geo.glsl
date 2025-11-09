#ifdef GL_ES
// OpenGL ES doesn't support geometry shaders
// This shader should not be used in ES contexts
// #error Geometry shaders are not supported in OpenGL ES
#else
// Desktop OpenGL versions

#if __VERSION__ >= 400
#define COMPAT_POS_IN(i) gl_in[i].gl_Position
layout(triangles) in;
layout(triangle_strip, max_vertices = 18) out;
uniform int layerOffset;
#define LAYER_OFFSET layerOffset
in float vColor[];
in vec2 texcoord[];
out vec4 FragPos;
out float vColorAlpha;
out vec2 texcoord0;
#elif __VERSION__ >= 150
#define COMPAT_POS_IN(i) gl_in[i].gl_Position
layout(triangles) in;
layout(triangle_strip, max_vertices = 18) out;
uniform int layerOffset;
#define LAYER_OFFSET layerOffset
in float vColor[];
in vec2 texcoord[];
out vec4 FragPos;
out float vColorAlpha;
out vec2 texcoord0;
#elif __VERSION__ >= 130
#extension GL_ARB_geometry_shader4 : enable
#define COMPAT_POS_IN(i) gl_in[i].gl_Position
layout(triangles) in;
layout(triangle_strip, max_vertices = 18) out;
uniform int layerOffset;
#define LAYER_OFFSET layerOffset
in float vColor[];
in vec2 texcoord[];
out vec4 FragPos;
out float vColorAlpha;
out vec2 texcoord0;
#else
#extension GL_EXT_geometry_shader4: enable
#extension GL_EXT_gpu_shader4: enable
#define COMPAT_POS_IN(i) gl_PositionIn[i]
#define LAYER_OFFSET 0

varying in float vColor[3];
varying in vec2 texcoord[3];
varying out vec4 FragPos;
varying out float vColorAlpha;
varying out vec2 texcoord0;
#endif

uniform int lightIndex;
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
uniform Light lights[4];

uniform mat4 lightMatrices[24];

const int LightType_None = 0;
const int LightType_Directional = 1;
const int LightType_Point = 2;
const int LightType_Spot = 3;

void main() {
    if(lights[lightIndex].type == LightType_Point){
        for(int face = 0; face < 6; ++face)
        {
            gl_Layer = LAYER_OFFSET+face; // built-in variable that specifies to which face we render.
            for(int i = 0; i < 3; ++i) // for each triangle vertex
            {
                FragPos = COMPAT_POS_IN(i);
                texcoord0 = texcoord[i];
                vColorAlpha = vColor[i];
                gl_Position = lightMatrices[lightIndex*6+face] * COMPAT_POS_IN(i);
                EmitVertex();
            }    
            EndPrimitive();
        }
    }else if(lights[lightIndex].type != LightType_None){
        gl_Layer = LAYER_OFFSET;
        for(int i = 0; i < 3; ++i) // for each triangle vertex
        {
            FragPos = COMPAT_POS_IN(i);
            texcoord0 = texcoord[i];
            vColorAlpha = vColor[i];
            gl_Position = lightMatrices[lightIndex*6] * COMPAT_POS_IN(i);
            EmitVertex();
        }
        EndPrimitive();
    }
}
#endif
