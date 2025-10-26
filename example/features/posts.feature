Feature: Posts
  Manage posts in the application

  Background: Setup
    Given document "posts" has the following items
      | id | title | content |
      | 1  | Post 1| Content 1 |

  Scenario: Create a new post
    When I send a POST request to "/posts" with body
      | title   | content   |
      | New Post| New Content |
    Then the response status should be 200
    And the response should contain an item with
      | title   | content   |
      | New Post| New Content |

  Scenario: Get a post
    When I send a GET request to "/posts/1"
    Then the response status should be 200
    And the response should contain an item with
      | id    | title  | content   |
      | 1     | Post 1 | Content 1 |
