precision highp float;

uniform sampler2D tex;
uniform vec4 baseColorFactor;
uniform vec3 add, mult;
uniform float gray, hue;
uniform bool textured;
uniform bool neg;
uniform bool enableAlpha;
uniform float alphaThreshold;

in vec2 texcoord;
in vec4 vColor;
out vec4 FragColor;

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
    vec4 fragColor;

    if (textured) {
        fragColor = texture(tex, texcoord) * baseColorFactor;
    } else {
        fragColor = baseColorFactor;
    }

    fragColor *= vec4(pow(vColor.r, 1.0/2.2), pow(vColor.g, 1.0/2.2), pow(vColor.b, 1.0/2.2), vColor.a);

    if (!enableAlpha) {
        if (fragColor.a < alphaThreshold) {
            discard;
        } else {
            fragColor.a = 1.0;
        }
    } else if (fragColor.a <= 0.0) {
        discard;
    }

    vec3 neg_base = vec3(1.0);
    neg_base *= fragColor.a;

    if (hue != 0.0) {
        fragColor.rgb = hue_shift(fragColor.rgb, hue);
    }

    if (neg) {
        fragColor.rgb = neg_base - fragColor.rgb;
    }

    fragColor.rgb = mix(fragColor.rgb, vec3((fragColor.r + fragColor.g + fragColor.b) / 3.0), gray) + add * fragColor.a;
    fragColor.rgb *= mult;

    FragColor = fragColor;
}