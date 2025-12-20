#ifdef GL_ES
layout(location = 0) out vec4 FragColor;
#else
out vec4 FragColor;
#endif

#ifdef GL_ES
// GLES path: geometry shader disabled => inputs come directly from vertex shader
in vec2 texcoord;
in vec4 vColor;
in vec3 fragPos;
in vec4 fragPosLight;
#else
// Desktop path: geometry shader active
in vec2 g_texcoord;
in vec4 g_vColor;
in vec3 g_fragPos;
in vec4 g_fragPosLight;
#endif

#ifdef GL_ES
uniform bool debugOutputColor;
#else
uniform bool debugOutputColor = false;
#endif

void main()
{
#ifdef GL_ES
    float ndcZ = fragPosLight.z / fragPosLight.w;
#else
    float ndcZ = g_fragPosLight.z / g_fragPosLight.w;
#endif

    float depth01 = clamp(ndcZ * 0.5 + 0.5, 0.0, 1.0);
    gl_FragDepth = depth01;

#ifdef GL_ES
    if (debugOutputColor) {
        FragColor = vec4(vec3(depth01), 1.0);
    } else {
        FragColor = vColor;
    }
#else
    if (debugOutputColor) {
        FragColor = vec4(vec3(depth01), 1.0);
    } else {
        FragColor = g_vColor;
    }
#endif
}
