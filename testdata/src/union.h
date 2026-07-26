typedef struct CommonEvent {
    unsigned int type;
    unsigned long long timestamp;
} CommonEvent;

typedef struct DisplayEvent {
    unsigned int type;
    unsigned long long timestamp;
    int data1;
    int data2;
} DisplayEvent;

typedef union Event {
    unsigned int type;
    CommonEvent common;
    DisplayEvent display;
} Event;

typedef union Value {
    long long num;
    double real;
    const char *str;
} Value;

union Slot {
    int index;
    Value value;
};

struct Item {
    int range;
    Value value;
};

int poll_event(Event *event);
double value_real(const Value *val);
