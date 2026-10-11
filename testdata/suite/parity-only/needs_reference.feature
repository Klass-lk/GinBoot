Feature: Scenarios that need a reference

  @parity
  Scenario: Fails unless skipped, since no reference is set
    When I send a GET request to "/notes" to both services
    Then both responses should match
