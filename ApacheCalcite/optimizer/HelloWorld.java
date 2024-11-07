package ApacheCalcite.optimizer;

import ApacheCalcite.example.*;

public class HelloWorld {
    public static void main(String[] args) {
        System.out.println("Hello, Woasasrld!");
        number num = new number();
        int val = num.sum(2, 3);
        System.out.println(val);
        Example ex = new Example();
        ex.print();
    }
}

class number {
    int sum(int a, int b) {
        return a + b;
    }
}