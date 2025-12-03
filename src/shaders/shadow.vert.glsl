// Inputs
layout(location = 0) in vec3 inPosition;
layout(location = 1) in vec3 inNormal;
layout(location = 2) in vec2 inTexcoord;
layout(location = 3) in vec4 inColor;

// Uniforms
uniform mat4 model;
uniform mat4 lightVP;

// Outputs to next stage
out vec2 texcoord;
out vec4 vColor;
out vec3 fragPos;
out vec4 fragPosLight;

void main()
{
    vec4 worldPos = model * vec4(inPosition, 1.0);

    fragPos = worldPos.xyz;
    fragPosLight = lightVP * worldPos;

    texcoord = inTexcoord;
    vColor = inColor;

#ifdef GL_ES
    // GLES uses gl_Position the same way OpenGL 3.3 does
#endif
    gl_Position = fragPosLight;
}
