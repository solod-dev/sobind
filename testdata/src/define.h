// A header whose API depends on a macro set by the caller.

#ifdef WITH_AUDIO
#define AUDIO_CHANNELS 2
void audio_open(int channels);
#endif

#ifndef WITH_AUDIO
void audio_unavailable(void);
#endif
