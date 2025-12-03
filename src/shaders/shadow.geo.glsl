#ifndef GL_ES
layout(triangles) in;
layout(triangle_strip, max_vertices = 3) out;

in vec2 texcoord[];      // arrays because geometry shader receives a primitive
in vec4 vColor[];
in vec3 fragPos[];
in vec4 fragPosLight[]; // clip-space positions from vertex shader

out vec2 g_texcoord;     // forwarded to fragment shader
out vec4 g_vColor;
out vec3 g_fragPos;
out vec4 g_fragPosLight;

void main()
{
    for(int i = 0; i < 3; ++i)
    {
        g_texcoord = texcoord[i];
        g_vColor   = vColor[i];
        g_fragPos  = fragPos[i];
        g_fragPosLight = fragPosLight[i];

        // gl_Position must be set to the same clip-space position used in vertex
        gl_Position = fragPosLight[i];

        EmitVertex();
    }
    EndPrimitive();
}
#endif