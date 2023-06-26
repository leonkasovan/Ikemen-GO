#version 320 es

precision mediump float;
precision mediump sampler2D;

uniform sampler2D Texture;
uniform vec2 TextureSize;
in vec2 v_TexCoord;
out vec4 FragColor;

void main(void) {
    vec4 rgb = texture(Texture, v_TexCoord);
    vec4 intens;
    if (fract(gl_FragCoord.y * (0.5*4.0/3.0)) > 0.5)
        intens = vec4(0.0);
    else
        intens = smoothstep(0.2, 0.8, rgb) + normalize(vec4(rgb.xyz, 1.0));
    float level = (4.0 - v_TexCoord.y) * 0.19;
    FragColor = intens * (0.5 - level) + rgb * 1.1;
}
